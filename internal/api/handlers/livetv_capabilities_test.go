package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeLiveTVClientCapabilities(t *testing.T) {
	body := `{"codecs_video":["h264","vp9"],"codecs_audio":["aac"],"max_audio_channels":2,"max_resolution":"1080p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/livetv/channels/ch1/session", strings.NewReader(body))

	caps, err := decodeLiveTVClientCapabilities(req)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(caps.CodecsVideo) != 2 || caps.CodecsVideo[0] != "h264" {
		t.Fatalf("codecs_video = %v", caps.CodecsVideo)
	}
	if caps.MaxAudioChannels != 2 || caps.MaxResolution != "1080p" {
		t.Fatalf("caps = %+v", caps)
	}
	if !caps.Declared() {
		t.Fatal("expected declared capabilities")
	}
}

// Clients released before capability reporting POST nothing (or an empty
// object); both must keep the copy path rather than fail the tune.
func TestDecodeLiveTVClientCapabilitiesOptional(t *testing.T) {
	for name, body := range map[string]string{
		"empty body":   "",
		"empty object": "{}",
		"whitespace":   "  \n",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/livetv/channels/ch1/session", strings.NewReader(body))
			caps, err := decodeLiveTVClientCapabilities(req)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if caps.Declared() {
				t.Fatalf("caps = %+v, want undeclared", caps)
			}
		})
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/livetv/channels/ch1/session", nil)
	req.Body = nil
	if _, err := decodeLiveTVClientCapabilities(req); err != nil {
		t.Fatalf("nil body decode: %v", err)
	}
}

func TestDecodeLiveTVClientCapabilitiesRejectsGarbage(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/livetv/channels/ch1/session", strings.NewReader("not json"))
	if _, err := decodeLiveTVClientCapabilities(req); err == nil {
		t.Fatal("expected a decode error")
	}
}
