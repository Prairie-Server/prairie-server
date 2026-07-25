package imageutil

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// imageutilWasm is the Rust WASI helper built from tools/imageutil-wasm.
// Provenance: imageutil.wasm.sha256.
//
//go:embed imageutil.wasm
var imageutilWasm []byte

const (
	guestInDir     = "/in"
	guestOutDir    = "/out"
	guestInputName = "src.bin"
)

const (
	defaultMaxSourceBytes = 64 << 20 // 64 MiB
	defaultTimeout        = 30 * time.Second
	defaultConcurrency    = 4
	defaultMaxMemoryPages = 8192 // 512 MiB
	defaultMaxLogBytes    = 64 << 10
)

// processorOptions configures the WASM image processor.
type processorOptions struct {
	MaxSourceBytes int64
	Timeout        time.Duration
	MaxConcurrent  int
	MaxMemoryPages uint32
	MaxLogBytes    int
	WorkRoot       string
}

func (o *processorOptions) applyDefaults() {
	if o.MaxSourceBytes <= 0 {
		o.MaxSourceBytes = defaultMaxSourceBytes
	}
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = defaultConcurrency
	}
	if o.MaxMemoryPages == 0 {
		o.MaxMemoryPages = defaultMaxMemoryPages
	}
	if o.MaxLogBytes <= 0 {
		o.MaxLogBytes = defaultMaxLogBytes
	}
}

type processor struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	opts     processorOptions
	sem      chan struct{}
	closed   atomic.Bool
}

type manifest struct {
	Ext      string            `json:"ext"`
	Variants []manifestVariant `json:"variants"`
}

type manifestVariant struct {
	Key  string `json:"key"`
	File string `json:"file"`
}

var (
	defaultProcessor     *processor
	defaultProcessorErr  error
	defaultProcessorOnce sync.Once
)

func getProcessor() (*processor, error) {
	defaultProcessorOnce.Do(func() {
		defaultProcessor, defaultProcessorErr = newProcessor(context.Background(), processorOptions{})
	})
	if defaultProcessorErr != nil {
		return nil, defaultProcessorErr
	}
	return defaultProcessor, nil
}

func newProcessor(ctx context.Context, opts processorOptions) (*processor, error) {
	opts.applyDefaults()
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(opts.MaxMemoryPages)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("imageutil: instantiate wasi: %w", err)
	}
	compiled, err := r.CompileModule(ctx, imageutilWasm)
	if err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("imageutil: compile module: %w", err)
	}
	return &processor{
		runtime:  r,
		compiled: compiled,
		opts:     opts,
		sem:      make(chan struct{}, opts.MaxConcurrent),
	}, nil
}

func (p *processor) close(ctx context.Context) error {
	if p == nil || p.runtime == nil {
		return nil
	}
	p.closed.Store(true)
	return p.runtime.Close(ctx)
}

func (p *processor) run(ctx context.Context, mode string, data []byte, extraArgs []string) (*VariantResult, error) {
	if p == nil || p.closed.Load() {
		return nil, fmt.Errorf("imageutil: processor unavailable")
	}
	if int64(len(data)) > p.opts.MaxSourceBytes {
		return nil, fmt.Errorf("imageutil: source exceeds %d bytes", p.opts.MaxSourceBytes)
	}

	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	scratch, err := os.MkdirTemp(p.opts.WorkRoot, "imageutil-")
	if err != nil {
		return nil, fmt.Errorf("imageutil: scratch: %w", err)
	}
	defer os.RemoveAll(scratch)

	inDir := filepath.Join(scratch, "in")
	outDir := filepath.Join(scratch, "out")
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return nil, fmt.Errorf("imageutil: in dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("imageutil: out dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(inDir, guestInputName), data, 0o644); err != nil {
		return nil, fmt.Errorf("imageutil: stage input: %w", err)
	}

	args := []string{
		"imageutil-wasm",
		"--mode", mode,
		"--input", guestInDir + "/" + guestInputName,
		"--outdir", guestOutDir,
	}
	args = append(args, extraArgs...)

	runCtx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()

	stdout := &cappedWriter{max: p.opts.MaxLogBytes}
	stderr := &cappedWriter{max: p.opts.MaxLogBytes}
	fsConfig := wazero.NewFSConfig().
		WithFSMount(os.DirFS(inDir), guestInDir).
		WithDirMount(outDir, guestOutDir)
	modConfig := wazero.NewModuleConfig().
		WithArgs(args...).
		WithFSConfig(fsConfig).
		WithStdout(stdout).
		WithStderr(stderr).
		WithSysWalltime().
		WithName("")

	mod, instErr := p.runtime.InstantiateModule(runCtx, p.compiled, modConfig)
	if mod != nil {
		_ = mod.Close(runCtx)
	}
	if err := classifyRunError(ctx, runCtx, instErr, p.opts.Timeout, stderr.String()); err != nil {
		return nil, err
	}

	var man manifest
	if err := json.Unmarshal(stdout.Bytes(), &man); err != nil {
		return nil, fmt.Errorf("imageutil: parse manifest: %w (stderr=%s)", err, trim(stderr.String()))
	}
	if man.Ext == "" || len(man.Variants) == 0 {
		return nil, fmt.Errorf("imageutil: empty manifest (stderr=%s)", trim(stderr.String()))
	}

	variants := make([]Variant, 0, len(man.Variants))
	for _, v := range man.Variants {
		if v.Key == "" || v.File == "" || strings.Contains(v.File, "..") || filepath.Base(v.File) != v.File {
			return nil, fmt.Errorf("imageutil: invalid manifest entry %+v", v)
		}
		raw, err := os.ReadFile(filepath.Join(outDir, v.File))
		if err != nil {
			return nil, fmt.Errorf("imageutil: read variant %s: %w", v.Key, err)
		}
		variants = append(variants, Variant{Key: v.Key, Data: raw})
	}
	return &VariantResult{Variants: variants, Ext: man.Ext}, nil
}

func classifyRunError(parent, run context.Context, instErr error, timeout time.Duration, stderr string) error {
	if instErr == nil {
		return nil
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(run.Err(), context.DeadlineExceeded) || errors.Is(instErr, context.DeadlineExceeded) {
		return fmt.Errorf("imageutil: timed out after %s", timeout)
	}
	if errors.Is(instErr, context.Canceled) {
		return context.Canceled
	}
	var exitErr *sys.ExitError
	if errors.As(instErr, &exitErr) {
		switch exitErr.ExitCode() {
		case sys.ExitCodeDeadlineExceeded:
			return fmt.Errorf("imageutil: timed out after %s", timeout)
		case sys.ExitCodeContextCanceled:
			return context.Canceled
		case 0:
			return nil
		default:
			return fmt.Errorf("imageutil: wasm exit %d: %s", exitErr.ExitCode(), trim(stderr))
		}
	}
	return fmt.Errorf("imageutil: wasm run: %w (%s)", instErr, trim(stderr))
}

type cappedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if w.max > 0 && w.buf.Len() >= w.max {
		return len(p), nil
	}
	if w.max > 0 && w.buf.Len()+len(p) > w.max {
		p = p[:w.max-w.buf.Len()]
	}
	return w.buf.Write(p)
}

func (w *cappedWriter) String() string { return w.buf.String() }
func (w *cappedWriter) Bytes() []byte  { return w.buf.Bytes() }

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return s[:512] + "…"
	}
	return s
}

func joinUintCSV(vals []int) string {
	if len(vals) == 0 {
		return ""
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
