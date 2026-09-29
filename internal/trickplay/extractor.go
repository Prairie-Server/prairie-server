package trickplay

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/prairie-server/prairie-server/internal/mediaprobe"
)

const (
	sheetExtractTimeoutSDR = 2 * time.Minute
	sheetExtractTimeoutHDR = 5 * time.Minute

	// keyframeProbeWindow is how much of a sheet's range is demuxed to judge
	// its keyframe spacing. An encode's GOP structure is effectively uniform,
	// so a short window stands in for the sheet without reading all of it.
	keyframeProbeWindow  = 60.0
	keyframeProbeTimeout = 30 * time.Second
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
	// ProbeKeyframes returns keyframe timestamps in [start, start+window);
	// nil uses ffprobe beside FFmpegPath.
	ProbeKeyframes func(ctx context.Context, ffprobePath, inputPath string, start, window float64) ([]float64, error)
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

	probe := opts.ProbeKeyframes
	if probe == nil {
		probe = probeKeyframeTimes
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, keyframeProbeTimeout)
	keyframes, probeErr := probe(probeCtx, mediaprobe.FFprobePathFromFFmpeg(ffmpegPath), opts.InputPath, opts.SheetStart, keyframeProbeWindow)
	probeCancel()
	keyframeOnly := probeErr == nil && keyframesDenseEnough(keyframes, interval)

	args := buildSheetExtractArgs(opts.InputPath, opts.SheetStart, interval, width, columns, rows, opts.ToneMap, keyframeOnly)
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
	keyframeOnly bool,
) []string {
	// bt2390 exists only in jellyfin-ffmpeg's tonemapx; stock ffmpeg's tonemap
	// rejects it. Tone mapping after fps and scale keeps it to one small frame
	// per tile instead of every decoded 4K frame.
	toneMapChain := ""
	if toneMap {
		toneMapChain = "tonemapx=tonemap=bt2390,zscale=p=bt709:t=bt709:m=bt709:r=tv,format=yuv420p,"
	}
	vf := fmt.Sprintf("fps=1/%g,scale=%d:-2,%stile=%dx%d", interval, width, toneMapChain, columns, rows)
	args := []string{"-hide_banner", "-loglevel", "error"}
	if keyframeOnly {
		// Decoding every frame of a 4K HEVC source in software overruns the
		// sheet timeout; keyframes alone are exact enough when they are dense.
		args = append(args, "-skip_frame", "nokey")
	}
	return append(args,
		"-ss", fmt.Sprintf("%.3f", sheetStart),
		"-i", inputPath,
		"-vf", vf,
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-",
	)
}

// keyframesDenseEnough reports whether decoding only keyframes still gives
// every tile its own frame close to its time: no gap between keyframes may
// exceed half a tile interval. Longer GOPs (common in WEB encodes, where 10s
// is typical) would make neighboring tiles repeat one keyframe, so those
// sheets decode every frame instead.
func keyframesDenseEnough(keyframes []float64, interval float64) bool {
	if len(keyframes) < 2 {
		return false
	}
	for i := 1; i < len(keyframes); i++ {
		if keyframes[i]-keyframes[i-1] > interval/2 {
			return false
		}
	}
	return true
}

// probeKeyframeTimes lists the video keyframe timestamps in a window by
// demuxing packets; nothing is decoded.
func probeKeyframeTimes(ctx context.Context, ffprobePath, inputPath string, start, window float64) ([]float64, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", fmt.Sprintf("%.3f%%+%.3f", start, window),
		"-show_entries", "packet=pts_time,flags",
		"-of", "csv=p=0",
		inputPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe keyframes: %w", err)
	}
	var times []float64
	for _, line := range strings.Split(string(out), "\n") {
		pts, flags, ok := strings.Cut(strings.TrimSpace(line), ",")
		if !ok || !strings.HasPrefix(flags, "K") {
			continue
		}
		if t, err := strconv.ParseFloat(pts, 64); err == nil {
			times = append(times, t)
		}
	}
	return times, nil
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
