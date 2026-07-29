package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A HEAD probe must not drag the segment body across the node boundary. The
// proxy used to hardcode GET upstream, so an existence check pulled a whole
// multi-megabyte segment only for net/http to discard it on the way out.
func TestProxyToTranscodeNodeForwardsRequestMethod(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			var upstreamMethod string
			var upstreamBodyWritten int
			node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamMethod = r.Method
				w.Header().Set("Content-Type", "video/iso.segment")
				n, _ := w.Write([]byte("segment-bytes"))
				upstreamBodyWritten = n
			}))
			defer node.Close()

			h := &PlaybackHandler{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/api/v1/playback/transcode/s1/segment/seg_00000.m4s", nil)

			h.proxyToTranscodeNode(rec, req, node.URL, "/transcode/s1/segment/seg_00000.m4s")

			if upstreamMethod != method {
				t.Errorf("upstream method = %q, want %q", upstreamMethod, method)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if method == http.MethodHead && upstreamBodyWritten != 0 {
				// The node's own server suppresses the body for HEAD; what matters
				// here is that we asked for HEAD rather than GET.
				t.Logf("node reported %d body bytes for HEAD", upstreamBodyWritten)
			}
		})
	}
}

// The node may serve GET only. A 405 must pass through untouched so the client
// falls back to a ranged GET instead of treating it as a hard failure.
func TestProxyToTranscodeNodePassesThroughMethodNotAllowed(t *testing.T) {
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer node.Close()

	h := &PlaybackHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/api/v1/playback/transcode/s1/segment/seg_00000.m4s", nil)

	h.proxyToTranscodeNode(rec, req, node.URL, "/transcode/s1/segment/seg_00000.m4s")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 passed through", rec.Code)
	}
}
