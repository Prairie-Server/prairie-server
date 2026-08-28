package diagnostics

import (
	"errors"
	"strings"
	"testing"
)

func TestNewShortIDIsParseable(t *testing.T) {
	id, err := NewShortID()
	if err != nil {
		t.Fatalf("NewShortID: %v", err)
	}
	if len(id) != len(shortIDPrefix)+shortIDPayloadLength {
		t.Fatalf("length = %d, want %d", len(id), len(shortIDPrefix)+shortIDPayloadLength)
	}
	if !strings.HasPrefix(id, shortIDPrefix) {
		t.Fatalf("id = %q, want %s prefix", id, shortIDPrefix)
	}
	if _, err := ParseShortID(id); err != nil {
		t.Fatalf("ParseShortID(%q): %v", id, err)
	}
}

func TestParseShortIDNormalizesCaseAndOptionalPrefix(t *testing.T) {
	for _, raw := range []string{"abcdef123456", "prairie-abcdef123456", " PRAIRIE-ABCDEF123456 "} {
		got, err := ParseShortID(raw)
		if err != nil {
			t.Fatalf("ParseShortID(%q): %v", raw, err)
		}
		if got != "PRAIRIE-ABCDEF123456" {
			t.Fatalf("ParseShortID(%q) = %q, want PRAIRIE-ABCDEF123456", raw, got)
		}
	}
}

func TestParseShortIDRejectsAmbiguousCharacters(t *testing.T) {
	for _, raw := range []string{"ABCDEF12345I", "ABCDEF12345L", "ABCDEF12345O", "ABCDEF12345U"} {
		if _, err := ParseShortID(raw); !errors.Is(err, ErrInvalidShortID) {
			t.Fatalf("ParseShortID(%q) error = %v, want ErrInvalidShortID", raw, err)
		}
	}
}

func TestParseShortIDAcceptsLegacySiloPrefix(t *testing.T) {
	got, err := ParseShortID("silo-abcdef123456")
	if err != nil {
		t.Fatalf("ParseShortID legacy: %v", err)
	}
	if got != "SILO-ABCDEF123456" {
		t.Fatalf("ParseShortID legacy = %q, want SILO-ABCDEF123456", got)
	}
}
