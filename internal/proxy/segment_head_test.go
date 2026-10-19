package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prairie-server/prairie-server/internal/streamtoken"
)

// A HEAD probe must not drag the segment body across the node boundary: the
// proxy forwards the request method rather than hardcoding GET upstream.
func TestProxyToTranscodeNodeForwardsRequestMethod(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			var upstreamMethod string
			node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamMethod = r.Method
				w.Header().Set("Content-Type", "video/iso.segment")
				_, _ = w.Write([]byte("segment-bytes"))
			}))
			defer node.Close()

			server := newCompletionTestProxy(node.Client())
			claims := &streamtoken.Claims{SessionID: "public", TranscodeNode: node.URL}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/stream/transcode/token/segment/seg_00000.m4s", nil)
			server.proxyToTranscodeNode(rec, req, claims, "/transcode/s1/segment/seg_00000.m4s", "")

			if upstreamMethod != method {
				t.Errorf("upstream method = %q, want %q", upstreamMethod, method)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
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

	server := newCompletionTestProxy(node.Client())
	claims := &streamtoken.Claims{SessionID: "public", TranscodeNode: node.URL}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/stream/transcode/token/segment/seg_00000.m4s", nil)
	server.proxyToTranscodeNode(rec, req, claims, "/transcode/s1/segment/seg_00000.m4s", "")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 passed through", rec.Code)
	}
}
