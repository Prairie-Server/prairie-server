package httpheaders

import (
	"net/http"
	"testing"
)

func TestGetPrefersPrairieHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set(LegacyName(HeaderDeviceID), "legacy")
	headers.Set(HeaderDeviceID, "prairie")

	if got := Get(headers, HeaderDeviceID); got != "prairie" {
		t.Fatalf("Get() = %q, want prairie", got)
	}
}

func TestGetFallsBackToLegacyHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set(LegacyName(HeaderDeviceID), "legacy")

	if got := Get(headers, HeaderDeviceID); got != "legacy" {
		t.Fatalf("Get() = %q, want legacy", got)
	}
}

func TestSetEmitsPrairieHeader(t *testing.T) {
	headers := http.Header{}

	Set(headers, HeaderClient, "web")

	if got := headers.Get(HeaderClient); got != "web" {
		t.Fatalf("Prairie header = %q, want web", got)
	}
	if got := headers.Get(LegacyName(HeaderClient)); got != "" {
		t.Fatalf("legacy header = %q, want empty", got)
	}
}
