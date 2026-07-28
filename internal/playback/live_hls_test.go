package playback

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeLiveHLSFakeFFmpeg(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

// ffmpeg can finish writing playable segments and exit in the same instant the
// readiness loop looks at it; that is a usable stream, not a failed tune.
func TestStartLiveHLSAcceptsPlaylistWrittenBeforeExit(t *testing.T) {
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

	session, err := StartLiveHLS(context.Background(), LiveHLSOpts{
		ID:         "finished",
		InputURL:   "http://127.0.0.1/auto/v4.1",
		OutputDir:  filepath.Join(t.TempDir(), "out"),
		FFmpegPath: ffmpeg,
	})
	if err != nil {
		t.Fatalf("StartLiveHLS = %v, want a ready session", err)
	}
	t.Cleanup(func() { _ = session.Close() })
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

	if err := os.WriteFile(filepath.Join(dir, "seg_00000.ts"), []byte("ts"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !liveHLSPlaylistReady(playlist) {
		t.Fatal("playlist with a non-empty .ts segment should be ready")
	}

	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:1.0,\nseg_00000.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !liveHLSPlaylistReady(playlist) {
		t.Fatal("playlist listing #EXTINF should be ready")
	}
}
