package intromarkers

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// fingerprintCompareWasm is the Rust WASI helper built from tools/fingerprint-compare-wasm.
// Provenance: fingerprint_compare.wasm.sha256.
//
//go:embed fingerprint_compare.wasm
var fingerprintCompareWasm []byte

const (
	guestInDir            = "/in"
	guestRequestName      = "request.json"
	defaultCompareTimeout = 60 * time.Second
	defaultMaxLogBytes    = 64 << 10
	defaultMaxMemoryPages = 512 // 32 MiB
)

type compareProcessor struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	timeout  time.Duration
	maxLog   int
	closed   atomic.Bool
}

type wasmCompareRequest struct {
	PointHopSeconds             float64                `json:"point_hop_seconds"`
	MinimumIntroDurationSeconds int                    `json:"minimum_intro_duration_seconds"`
	MaximumIntroDurationSeconds int                    `json:"maximum_intro_duration_seconds"`
	Inputs                      []wasmFingerprintInput `json:"inputs"`
}

type wasmFingerprintInput struct {
	Index     int      `json:"index"`
	EpisodeID string   `json:"episode_id"`
	Points    []uint32 `json:"points"`
}

type wasmCompareResponse struct {
	Matches []wasmPairMatch `json:"matches"`
}

type wasmPairMatch struct {
	LeftIndex  int         `json:"left_index"`
	RightIndex int         `json:"right_index"`
	Left       wasmSegment `json:"left"`
	Right      wasmSegment `json:"right"`
}

type wasmSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

var (
	defaultCompareProcessor     *compareProcessor
	defaultCompareProcessorErr  error
	defaultCompareProcessorOnce sync.Once
)

func getCompareProcessor() (*compareProcessor, error) {
	defaultCompareProcessorOnce.Do(func() {
		defaultCompareProcessor, defaultCompareProcessorErr = newCompareProcessor(context.Background())
	})
	if defaultCompareProcessorErr != nil {
		return nil, defaultCompareProcessorErr
	}
	return defaultCompareProcessor, nil
}

func newCompareProcessor(ctx context.Context) (*compareProcessor, error) {
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(defaultMaxMemoryPages)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("intromarkers: instantiate wasi: %w", err)
	}
	compiled, err := r.CompileModule(ctx, fingerprintCompareWasm)
	if err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("intromarkers: compile fingerprint compare module: %w", err)
	}
	return &compareProcessor{
		runtime:  r,
		compiled: compiled,
		timeout:  defaultCompareTimeout,
		maxLog:   defaultMaxLogBytes,
	}, nil
}

func (p *compareProcessor) compare(ctx context.Context, inputs []fingerprintInput, cfg Config) ([]wasmPairMatch, error) {
	if p == nil || p.closed.Load() {
		return nil, fmt.Errorf("intromarkers: fingerprint compare processor unavailable")
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	scratch, err := os.MkdirTemp("", "fingerprint-compare-")
	if err != nil {
		return nil, fmt.Errorf("intromarkers: scratch: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	inDir := filepath.Join(scratch, "in")
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return nil, fmt.Errorf("intromarkers: in dir: %w", err)
	}

	req := wasmCompareRequest{
		PointHopSeconds:             DefaultPointHopSeconds,
		MinimumIntroDurationSeconds: cfg.MinimumIntroDurationSeconds,
		MaximumIntroDurationSeconds: cfg.MaximumIntroDurationSeconds,
		Inputs:                      make([]wasmFingerprintInput, len(inputs)),
	}
	for i, input := range inputs {
		req.Inputs[i] = wasmFingerprintInput{
			Index:     i,
			EpisodeID: input.Candidate.EpisodeID,
			Points:    input.Points,
		}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("intromarkers: encode compare request: %w", err)
	}
	if err := os.WriteFile(filepath.Join(inDir, guestRequestName), raw, 0o644); err != nil {
		return nil, fmt.Errorf("intromarkers: stage compare request: %w", err)
	}

	args := []string{
		"fingerprint-compare-wasm",
		"--input", guestInDir + "/" + guestRequestName,
	}

	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	stdout := &cappedWriter{max: p.maxLog}
	stderr := &cappedWriter{max: p.maxLog}
	fsConfig := wazero.NewFSConfig().WithFSMount(os.DirFS(inDir), guestInDir)
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
	if err := classifyCompareRunError(ctx, runCtx, instErr, p.timeout, stderr.String()); err != nil {
		return nil, err
	}

	var resp wasmCompareResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("intromarkers: parse compare response: %w (stderr=%s)", err, trimCompareLog(stderr.String()))
	}
	return resp.Matches, nil
}

func classifyCompareRunError(parent, run context.Context, instErr error, timeout time.Duration, stderr string) error {
	if instErr == nil {
		return nil
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(run.Err(), context.DeadlineExceeded) || errors.Is(instErr, context.DeadlineExceeded) {
		return fmt.Errorf("intromarkers: fingerprint compare timed out after %s", timeout)
	}
	if errors.Is(instErr, context.Canceled) {
		return context.Canceled
	}
	var exitErr *sys.ExitError
	if errors.As(instErr, &exitErr) {
		switch exitErr.ExitCode() {
		case sys.ExitCodeDeadlineExceeded:
			return fmt.Errorf("intromarkers: fingerprint compare timed out after %s", timeout)
		case sys.ExitCodeContextCanceled:
			return context.Canceled
		case 0:
			return nil
		default:
			return fmt.Errorf("intromarkers: fingerprint compare wasm exit %d: %s", exitErr.ExitCode(), trimCompareLog(stderr))
		}
	}
	return fmt.Errorf("intromarkers: fingerprint compare wasm run: %w (%s)", instErr, trimCompareLog(stderr))
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

func trimCompareLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return s[:512] + "…"
	}
	return s
}
