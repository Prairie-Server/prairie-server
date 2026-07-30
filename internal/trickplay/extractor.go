package trickplay

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	sheetExtractTimeoutSDR = 2 * time.Minute
	sheetExtractTimeoutHDR = 5 * time.Minute
)

type SheetExtractOptions struct {
	InputPath       string
	SheetStart      float64
	IntervalSeconds float64
	TileWidth       int
	TileColumns     int
	TileRows        int
	FFmpegPath      string
	ToneMap         bool
	RunFunc         func(ctx context.Context, ffmpegPath string, args []string) ([]byte, error)
}

func ExtractSheet(ctx context.Context, opts SheetExtractOptions) ([]byte, string, error) {
	ffmpegPath := opts.FFmpegPath
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	runExtract := opts.RunFunc
	if runExtract == nil {
		runExtract = runFFmpegSheetExtract
	}

	interval := opts.IntervalSeconds
	if interval <= 0 {
		interval = DefaultIntervalSeconds
	}
	width := opts.TileWidth
	if width <= 0 {
		width = DefaultTileWidth
	}
	columns := opts.TileColumns
	if columns <= 0 {
		columns = DefaultTileColumns
	}
	rows := opts.TileRows
	if rows <= 0 {
		rows = DefaultTileRows
	}

	args := buildSheetExtractArgs(opts.InputPath, opts.SheetStart, interval, width, columns, rows, opts.ToneMap)
	timeout := sheetExtractTimeoutSDR
	if opts.ToneMap {
		timeout = sheetExtractTimeoutHDR
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	data, err := runExtract(attemptCtx, ffmpegPath, args)
	if err != nil {
		reason := classifySheetExtractError(err)
		return nil, reason, wrapReason(reason, err)
	}
	return data, "", nil
}

func buildSheetExtractArgs(
	inputPath string,
	sheetStart float64,
	interval float64,
	width, columns, rows int,
	toneMap bool,
) []string {
	vf := fmt.Sprintf("fps=1/%g,scale=%d:-2,tile=%dx%d", interval, width, columns, rows)
	if toneMap {
		vf = "zscale=t=linear:npl=100,format=gbrpf32le,tonemap=bt2390,zscale=p=bt709:t=bt709:m=bt709:r=tv,format=yuv420p," + vf
	}
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-ss", fmt.Sprintf("%.3f", sheetStart),
		"-i", inputPath,
		"-vf", vf,
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-",
	}
}

func runFFmpegSheetExtract(ctx context.Context, ffmpegPath string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg extract sheet: %w (%s)", err, stderr.String())
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg extract sheet: empty output")
	}
	return stdout.Bytes(), nil
}

func classifySheetExtractError(err error) string {
	if err == nil {
		return "sheet_extract_failed"
	}
	if isDeadlineError(err) {
		return "sheet_extract_timeout"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "Invalid data found when processing input"):
		return "decode_invalid_data"
	case strings.Contains(message, "No such filter") || (strings.Contains(message, "tonemap") && strings.Contains(message, "Error")):
		return "tonemap_unsupported"
	case strings.Contains(message, "No such file"):
		return "input_missing"
	default:
		return "sheet_extract_failed"
	}
}

func isDeadlineError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), context.DeadlineExceeded.Error()) || strings.Contains(err.Error(), "signal: killed"))
}

func wrapReason(reason string, err error) error {
	if err == nil || reason == "" {
		return err
	}
	return fmt.Errorf("%s: %w", reason, err)
}
