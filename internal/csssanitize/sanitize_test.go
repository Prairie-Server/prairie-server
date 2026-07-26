package csssanitize

import (
	"strings"
	"testing"
)

func TestSanitizeBlocksExternalResources(t *testing.T) {
	t.Parallel()
	in := `@import url("https://evil.example/x.css");
.shell { color: red; background: url(https://evil.example/bg.png); }
.ok { background: url(/assets/a.png); font: url(data:font/woff2;base64,AA); }`
	out := Sanitize(in)
	if strings.Contains(out, "evil.example") {
		t.Fatalf("external host leaked: %s", out)
	}
	if !strings.Contains(out, "/assets/a.png") || !strings.Contains(out, "data:font") {
		t.Fatalf("safe urls stripped: %s", out)
	}
}
