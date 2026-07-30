package livetv

import "testing"

// The delivery ID is what a stream token gets bound to, so parsing it wrong
// would either mint an unusable token or bind one to the wrong session.
func TestLiveHLSDeliveryID(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"built path round-trips", PublicLiveHLSPath("abc123"), "abc123"},
		{"segment path", "/api/v1/livetv/live-hls/abc123/seg_00007.ts", "abc123"},
		{"already carries a query", "/api/v1/livetv/live-hls/abc123/index.m3u8?profile_id=p1", "abc123"},
		{"fragment is not part of the id", "/api/v1/livetv/live-hls/abc123/index.m3u8#x", "abc123"},
		{"absolute url", "https://host/api/v1/livetv/live-hls/abc123/index.m3u8", "abc123"},
		// The MPEG-TS session proxy is keyed on a different ID, so it must not
		// match -- binding a token to the wrong ID would fail verification.
		{"session stream path does not match", PublicSessionStreamPath("abc123"), ""},
		{"unrelated path", "/api/v1/stream/abc123", ""},
		{"missing name", "/api/v1/livetv/live-hls/abc123", ""},
		{"empty id", "/api/v1/livetv/live-hls//index.m3u8", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := LiveHLSDeliveryID(tc.url)
			if tc.want == "" {
				if ok {
					t.Errorf("LiveHLSDeliveryID(%q) matched, want no match (got %q)", tc.url, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("LiveHLSDeliveryID(%q) = (%q, %v), want (%q, true)", tc.url, got, ok, tc.want)
			}
		})
	}
}

// The builder and the parser share one prefix constant precisely so they cannot
// disagree; this pins that they actually do.
func TestPublicLiveHLSPathIsParseable(t *testing.T) {
	const id = "136434354095130390"
	got, ok := LiveHLSDeliveryID(PublicLiveHLSPath(id))
	if !ok || got != id {
		t.Fatalf("round trip failed: got (%q, %v), want (%q, true)", got, ok, id)
	}
}
