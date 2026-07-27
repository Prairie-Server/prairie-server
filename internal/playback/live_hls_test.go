package playback

import (
	"os"
	"path/filepath"
	"testing"
)

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
