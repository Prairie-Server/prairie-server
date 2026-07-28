package livetv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFFmpegRecordingArgs writes its own command line next to the playlist so a
// test can assert what the bridge asked ffmpeg to do.
const fakeFFmpegRecordingArgs = `#!/bin/sh
outdir=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-hls_segment_filename" ]; then
    outdir=$(dirname "$arg")
  fi
  prev="$arg"
done
if [ -n "$outdir" ]; then
  mkdir -p "$outdir"
  echo "$@" > "$outdir/args.txt"
  # Three segments so encoding sessions clear DefaultLiveTranscodeLeadSegments.
  printf '#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n' > "$outdir/index.m3u8"
  for i in 0 1 2; do
    printf 'seg' > "$outdir/seg_0000$i.ts"
    printf '#EXTINF:1.0,\nseg_0000%s.ts\n' "$i" >> "$outdir/index.m3u8"
  done
fi
while true; do sleep 1; done
`

func readBridgeFFmpegArgs(t *testing.T, root, playbackID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "livetv-hls", playbackID, "args.txt"))
	if err != nil {
		t.Fatalf("read ffmpeg args: %v", err)
	}
	return string(data)
}

func TestHLSBridgeEncodesForBrowserPlan(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegRecordingArgs)
	bridge := NewHLSBridge(HLSBridgeOptions{Root: root, FFmpegPath: ffmpeg, HWAccel: "none"})
	ctx := context.Background()

	id, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch1",
		SourceURL: "http://127.0.0.1/auto/v4.1",
		UserID:    7,
		ProfileID: "prof",
		Plan:      StreamPlan{VideoCodec: "h264", AudioCodec: "aac"},
	})
	if err != nil {
		t.Fatalf("StartLiveStream: %v", err)
	}
	t.Cleanup(func() { _ = bridge.StopLiveStream(ctx, id) })

	args := readBridgeFFmpegArgs(t, root, id)
	if !strings.Contains(args, "-c:v libx264") {
		t.Fatalf("expected an h264 encode, got: %s", args)
	}
	if !strings.Contains(args, "-c:a aac") {
		t.Fatalf("expected an AAC encode, got: %s", args)
	}
}

func TestHLSBridgeCopiesForNativePlan(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegRecordingArgs)
	bridge := NewHLSBridge(HLSBridgeOptions{Root: root, FFmpegPath: ffmpeg})
	ctx := context.Background()

	id, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch1",
		SourceURL: "http://127.0.0.1/auto/v4.1",
	})
	if err != nil {
		t.Fatalf("StartLiveStream: %v", err)
	}
	t.Cleanup(func() { _ = bridge.StopLiveStream(ctx, id) })

	args := readBridgeFFmpegArgs(t, root, id)
	if !strings.Contains(args, "-c:v copy") || !strings.Contains(args, "-c:a copy") {
		t.Fatalf("expected a straight remux, got: %s", args)
	}
}

// Encodes are the expensive sessions, so they are capped; copies are not.
func TestHLSBridgeLimitsConcurrentTranscodes(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegRecordingArgs)
	bridge := NewHLSBridge(HLSBridgeOptions{Root: root, FFmpegPath: ffmpeg, HWAccel: "none", MaxTranscodes: 1})
	ctx := context.Background()
	encodePlan := StreamPlan{VideoCodec: "h264", AudioCodec: "aac"}

	first, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch1", SourceURL: "http://127.0.0.1/auto/v4.1", Plan: encodePlan,
	})
	if err != nil {
		t.Fatalf("first transcode: %v", err)
	}

	if _, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch2", SourceURL: "http://127.0.0.1/auto/v5.1", Plan: encodePlan,
	}); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("second transcode = %v, want ErrLimitExceeded", err)
	}

	// A copy session is cheap and must not be blocked by the encode cap.
	copyID, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch3", SourceURL: "http://127.0.0.1/auto/v6.1",
	})
	if err != nil {
		t.Fatalf("copy session while transcodes are full: %v", err)
	}
	_ = bridge.StopLiveStream(ctx, copyID)

	// Stopping the encode returns its slot.
	if err := bridge.StopLiveStream(ctx, first); err != nil {
		t.Fatalf("StopLiveStream: %v", err)
	}
	third, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch4", SourceURL: "http://127.0.0.1/auto/v7.1", Plan: encodePlan,
	})
	if err != nil {
		t.Fatalf("transcode after release: %v", err)
	}
	_ = bridge.StopLiveStream(ctx, third)
}

func TestHLSBridgeUnlimitedTranscodes(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegRecordingArgs)
	bridge := NewHLSBridge(HLSBridgeOptions{Root: root, FFmpegPath: ffmpeg, HWAccel: "none", MaxTranscodes: -1})
	ctx := context.Background()
	encodePlan := StreamPlan{VideoCodec: "h264"}

	for _, channel := range []string{"ch1", "ch2", "ch3", "ch4"} {
		id, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
			ChannelID: channel, SourceURL: "http://127.0.0.1/auto/" + channel, Plan: encodePlan,
		})
		if err != nil {
			t.Fatalf("start %s: %v", channel, err)
		}
		t.Cleanup(func() { _ = bridge.StopLiveStream(ctx, id) })
	}
}

// Settings are read per tune so an admin change lands on the next channel
// start instead of waiting for a restart.
func TestHLSBridgeAppliesSettingsPerTune(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegRecordingArgs)
	settings := TranscodeSettings{HWAccel: "none", PlayMethod: PlayMethodAuto}
	bridge := NewHLSBridge(HLSBridgeOptions{
		Root:       root,
		FFmpegPath: ffmpeg,
		Settings:   func(context.Context) TranscodeSettings { return settings },
	})
	ctx := context.Background()
	browserPlan := StreamPlan{VideoCodec: "h264", AudioCodec: "aac"}

	first, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch1", SourceURL: "http://127.0.0.1/auto/v4.1", Plan: browserPlan,
	})
	if err != nil {
		t.Fatalf("StartLiveStream: %v", err)
	}
	if args := readBridgeFFmpegArgs(t, root, first); !strings.Contains(args, "-c:v libx264") {
		t.Fatalf("expected an encode, got: %s", args)
	}
	_ = bridge.StopLiveStream(ctx, first)

	// Operator forces copy; the next tune must respect it without a restart.
	settings.PlayMethod = PlayMethodCopy
	second, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
		ChannelID: "ch1", SourceURL: "http://127.0.0.1/auto/v4.1", Plan: browserPlan,
	})
	if err != nil {
		t.Fatalf("StartLiveStream after settings change: %v", err)
	}
	t.Cleanup(func() { _ = bridge.StopLiveStream(ctx, second) })
	args := readBridgeFFmpegArgs(t, root, second)
	if !strings.Contains(args, "-c:v copy") {
		t.Fatalf("expected forced copy, got: %s", args)
	}
	if strings.Contains(args, "libx264") {
		t.Fatalf("forced copy must not encode: %s", args)
	}
}

// A forced copy is cheap, so it must not consume an encode slot.
func TestHLSBridgeForcedCopySkipsTranscodeLimit(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, fakeFFmpegRecordingArgs)
	bridge := NewHLSBridge(HLSBridgeOptions{
		Root:       root,
		FFmpegPath: ffmpeg,
		Settings: func(context.Context) TranscodeSettings {
			return TranscodeSettings{PlayMethod: PlayMethodCopy, MaxTranscodes: 1}
		},
	})
	ctx := context.Background()
	browserPlan := StreamPlan{VideoCodec: "h264", AudioCodec: "aac"}

	for _, channel := range []string{"ch1", "ch2", "ch3"} {
		id, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
			ChannelID: channel, SourceURL: "http://127.0.0.1/auto/" + channel, Plan: browserPlan,
		})
		if err != nil {
			t.Fatalf("forced-copy tune %s: %v", channel, err)
		}
		t.Cleanup(func() { _ = bridge.StopLiveStream(ctx, id) })
	}
}

// A failed start must not strand the encode slot it reserved.
func TestHLSBridgeReleasesSlotWhenStartFails(t *testing.T) {
	allowLoopbackMediaFetch(t)
	root := t.TempDir()
	ffmpeg := writeFakeFFmpeg(t, root, "#!/bin/sh\nexit 1\n")
	bridge := NewHLSBridge(HLSBridgeOptions{Root: root, FFmpegPath: ffmpeg, HWAccel: "none", MaxTranscodes: 1})
	ctx := context.Background()
	encodePlan := StreamPlan{VideoCodec: "h264"}

	for i := 0; i < 3; i++ {
		if _, _, err := bridge.StartLiveStream(ctx, LiveStreamRequest{
			ChannelID: "ch1", SourceURL: "http://127.0.0.1/auto/v4.1", Plan: encodePlan,
		}); err == nil {
			t.Fatal("expected ffmpeg start failure")
		} else if errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("attempt %d leaked the transcode slot", i)
		}
	}
}
