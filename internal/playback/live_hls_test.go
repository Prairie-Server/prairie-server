package playback

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeLiveHLSFakeFFmpeg(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

// ffmpeg exiting after writing a playlist is not a usable live session: the
// sliding window will never advance, and the player ends up re-appending the
// same fragments until MSE fatals.
func TestStartLiveHLSFailsWhenEncoderExits(t *testing.T) {
	ffmpeg := writeLiveHLSFakeFFmpeg(t, `#!/bin/sh
outdir=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-hls_segment_filename" ]; then
    outdir=$(dirname "$arg")
  fi
  prev="$arg"
done
mkdir -p "$outdir"
printf 'seg' > "$outdir/seg_00000.ts"
printf '#EXTM3U\n#EXTINF:1.0,\nseg_00000.ts\n' > "$outdir/index.m3u8"
exit 0
`)

	if _, err := StartLiveHLS(context.Background(), LiveHLSOpts{
		ID:         "finished",
		InputURL:   "http://127.0.0.1/auto/v4.1",
		OutputDir:  filepath.Join(t.TempDir(), "out"),
		FFmpegPath: ffmpeg,
	}); err == nil {
		t.Fatal("expected an error when ffmpeg exits after writing a playlist")
	}
}

// A run that dies without producing anything is still a failed tune.
func TestStartLiveHLSFailsWhenNothingIsWritten(t *testing.T) {
	ffmpeg := writeLiveHLSFakeFFmpeg(t, "#!/bin/sh\nexit 1\n")

	if _, err := StartLiveHLS(context.Background(), LiveHLSOpts{
		ID:         "dead",
		InputURL:   "http://127.0.0.1/auto/v4.1",
		OutputDir:  filepath.Join(t.TempDir(), "out"),
		FFmpegPath: ffmpeg,
	}); err == nil {
		t.Fatal("expected an error when ffmpeg produces no playlist")
	}
}

// StartLiveHLS must not tie ffmpeg lifetime to the caller context. The HTTP
// handler cancels its request as soon as the start response is sent; the
// encoder has to keep running until LiveHLSSession.Close().
func TestStartLiveHLSSurvivesParentContextCancel(t *testing.T) {
	ffmpeg := writeLiveHLSFakeFFmpeg(t, `#!/bin/sh
outdir=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-hls_segment_filename" ]; then
    outdir=$(dirname "$arg")
  fi
  prev="$arg"
done
mkdir -p "$outdir"
printf 'seg' > "$outdir/seg_00000.ts"
printf '#EXTM3U\n#EXTINF:1.0,\nseg_00000.ts\n' > "$outdir/index.m3u8"
while true; do sleep 1; done
`)

	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	session, err := StartLiveHLS(parent, LiveHLSOpts{
		ID:           "survive",
		InputURL:     "http://127.0.0.1/auto/v4.1",
		OutputDir:    filepath.Join(t.TempDir(), "out"),
		FFmpegPath:   ffmpeg,
		LeadSegments: 1,
	})
	if err != nil {
		t.Fatalf("StartLiveHLS = %v, want a ready session", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	cancel()
	select {
	case <-session.Done():
		t.Fatal("ffmpeg exited when parent context was cancelled")
	default:
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("ffmpeg still running after Close")
	}
}

func TestStartLiveHLSAbortsWhenParentCancelledBeforeReady(t *testing.T) {
	ffmpeg := writeLiveHLSFakeFFmpeg(t, `#!/bin/sh
while true; do sleep 1; done
`)

	parent, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	if _, err := StartLiveHLS(parent, LiveHLSOpts{
		ID:           "abort",
		InputURL:     "http://127.0.0.1/auto/v4.1",
		OutputDir:    filepath.Join(t.TempDir(), "out"),
		FFmpegPath:   ffmpeg,
		LeadSegments: 3,
	}); err == nil {
		t.Fatal("expected error when parent context cancelled before ready")
	}
}

func TestLiveHLSPlaylistReady(t *testing.T) {
	dir := t.TempDir()
	playlist := filepath.Join(dir, "index.m3u8")

	if liveHLSPlaylistReady(playlist) {
		t.Fatal("missing playlist should not be ready")
	}

	if err := os.WriteFile(playlist, []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if liveHLSPlaylistReady(playlist) {
		t.Fatal("empty media playlist without segments should not be ready")
	}

	// A .ts file on disk is not enough: clients can only fetch what the
	// playlist advertises, so readiness follows #EXTINF only.
	if err := os.WriteFile(filepath.Join(dir, "seg_00000.ts"), []byte("ts"), 0o644); err != nil {
		t.Fatal(err)
	}
	if liveHLSPlaylistReady(playlist) {
		t.Fatal("unadvertised .ts files must not make the playlist ready")
	}

	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:1.0,\nseg_00000.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !liveHLSPlaylistReady(playlist) {
		t.Fatal("playlist listing #EXTINF should be ready")
	}
}
