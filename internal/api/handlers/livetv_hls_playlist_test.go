package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteHLSPlaylistAuthQuery(t *testing.T) {
	in := `#EXTM3U
#EXT-X-TARGETDURATION:2
#EXTINF:2.0,
seg_00000.ts
#EXTINF:2.0,
seg_00001.ts?foo=1
https://cdn.example/abs.ts
`
	out := rewriteHLSPlaylistAuthQuery(in, "token=abc&profile_id=p1")
	if !strings.Contains(out, "seg_00000.ts?profile_id=p1&token=abc") &&
		!strings.Contains(out, "seg_00000.ts?token=abc&profile_id=p1") {
		t.Fatalf("missing auth on first segment:\n%s", out)
	}
	if !strings.Contains(out, "seg_00001.ts?") || !strings.Contains(out, "foo=1") ||
		!strings.Contains(out, "token=abc") || !strings.Contains(out, "profile_id=p1") {
		t.Fatalf("missing merged query on second segment:\n%s", out)
	}
	if !strings.Contains(out, "https://cdn.example/abs.ts\n") && !strings.HasSuffix(strings.TrimSpace(out), "https://cdn.example/abs.ts") {
		// absolute URL must stay untouched
		if strings.Contains(out, "https://cdn.example/abs.ts?") {
			t.Fatalf("absolute URL was rewritten:\n%s", out)
		}
	}
}

func TestServeLiveHLSPlaylistPropagatesQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.m3u8")
	if err := os.WriteFile(path, []byte("#EXTM3U\nseg_00000.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/livetv/live-hls/x/index.m3u8?token=t&profile_id=p", nil)
	rr := httptest.NewRecorder()
	serveLiveHLSPlaylist(rr, req, path)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "seg_00000.ts?") || !strings.Contains(body, "token=t") || !strings.Contains(body, "profile_id=p") {
		t.Fatalf("body=%q", body)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Fatalf("content-type=%q", ct)
	}
}
