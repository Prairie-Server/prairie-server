package playback

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Browser Live TV failed with hls.js bufferAppendError when segment 0 had
// video but no audio (AAC priming / late first audio packet). This runs the
// real encode args against a source whose audio is delayed past the first
// segment boundary and asserts seg_00000.ts still carries an AAC track.
func TestLiveHLSEncodedFirstSegmentContainsAudio(t *testing.T) {
	if testing.Short() {
		t.Skip("real FFmpeg integration test")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath := ffprobePathFromFFmpeg(ffmpegPath)
	if _, err := exec.LookPath(ffprobePath); err != nil {
		t.Skip("ffprobe is not installed beside ffmpeg")
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "delayed-audio.ts")
	// Video from t=0, audio delayed 1.5s so a naive mux leaves segment 0
	// (hls_time=1) without audio packets — the exact hls.js failure mode.
	genCtx, cancelGen := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelGen()
	gen := exec.CommandContext(genCtx, ffmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-filter_complex", "[1:a]adelay=1500|1500[a]",
		"-map", "0:v:0", "-map", "[a]",
		"-t", "4",
		"-c:v", "mpeg2video", "-b:v", "1000k",
		"-c:a", "ac3", "-b:a", "192k",
		"-f", "mpegts", "-y", sourcePath,
	)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate delayed-audio source: %v\n%s", err, out)
	}

	outDir := filepath.Join(dir, "hls")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	playlist := filepath.Join(outDir, "index.m3u8")
	segPattern := filepath.Join(outDir, "seg_%05d.ts")
	args, _ := buildLiveHLSArgs(LiveHLSOpts{
		InputURL:         sourcePath,
		VideoCodec:       "h264",
		AudioCodec:       "aac",
		SourceVideoCodec: "mpeg2video",
		SourceAudioCodec: "ac3",
		HWAccel:          "none",
	}, playlist, segPattern, 1, 6)

	encCtx, cancelEnc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelEnc()
	enc := exec.CommandContext(encCtx, ffmpegPath, args...)
	if out, err := enc.CombinedOutput(); err != nil {
		t.Fatalf("live HLS encode: %v\n%s", err, out)
	}

	firstSeg := filepath.Join(outDir, "seg_00000.ts")
	if info, err := os.Stat(firstSeg); err != nil || info.Size() == 0 {
		t.Fatalf("missing or empty first segment: %v", err)
	}

	probe := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name",
		"-of", "csv=p=0",
		firstSeg,
	)
	out, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe first segment: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "video") {
		t.Fatalf("first segment missing video stream:\n%s", text)
	}
	if !strings.Contains(text, "audio") {
		t.Fatalf("first segment missing audio stream (bufferAppendError regression):\n%s", text)
	}
	if !strings.Contains(text, "aac") {
		t.Fatalf("first segment audio is not AAC:\n%s", text)
	}
}
