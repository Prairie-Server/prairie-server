package imageutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedWasmMatchesRecordedSHA256(t *testing.T) {
	raw, err := os.ReadFile("imageutil.wasm.sha256")
	if err != nil {
		t.Fatalf("read sha256 sidecar: %v", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		t.Fatal("imageutil.wasm.sha256 is empty")
	}
	want := strings.ToLower(fields[0])

	sum := sha256.Sum256(imageutilWasm)
	got := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("embedded imageutil.wasm sha256 = %s, recorded = %s — artifact and provenance disagree", got, want)
	}
}
