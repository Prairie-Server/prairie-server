package artworkkey

import "testing"

func TestArtworkKeyEdgeCases(t *testing.T) {
	if Build("", "original", "", "") != "" || Build("base", "", "", "") != "" {
		t.Fatal("empty build")
	}
	if got := Build("base", "original", "", "png"); got != "base/original.png" {
		t.Fatalf("ext without dot: %q", got)
	}
	if got := Build("base/", "w500", "rev", ""); got != "base/w500.rev.webp" {
		t.Fatalf("default ext: %q", got)
	}
	if Variant("", "w500") != "" {
		t.Fatal("empty original")
	}
	if got := Variant("x", ""); got != "x" {
		t.Fatalf("empty variant passthrough: %q", got)
	}
	if got := Variant("weird/path", "w500"); got != "weird/path" {
		t.Fatalf("unrecognized: %q", got)
	}
	if Directory("") != "" || Directory("https://x/y.webp") != "" || Directory("file.webp") != "" {
		t.Fatal("directory empty cases")
	}
	if Revision("noext") != "" || Revision("original.") != "" {
		t.Fatal("revision empty")
	}
	if len(VariantWidths("backdrop")) != 3 || len(VariantWidths("logo")) != 1 || len(VariantWidths("poster")) != 2 {
		t.Fatal("widths")
	}
	if ObjectKeys("", "poster") != nil || ObjectKeys("https://x/y.webp", "poster") != nil {
		t.Fatal("object keys nil")
	}
	if FormatSibling("", ".avif") != "" {
		t.Fatal("format empty")
	}
	if FormatSibling("path/noext", ".avif") != "path/noext" {
		t.Fatal("no ext passthrough")
	}
	if got := FormatSibling("a.webp", "avif"); got != "a.avif" {
		t.Fatalf("ext without dot: %q", got)
	}
	if got := FormatSibling("https://cdn/x.webp?q=1#f", ".avif"); got != "https://cdn/x.avif?q=1#f" {
		t.Fatalf("url sibling: %q", got)
	}
	if FormatSibling("https://cdn/noext", ".avif") != "https://cdn/noext" {
		t.Fatal("url no ext")
	}
	if got := FormatSibling("://bad", ".avif"); got != "://bad" {
		t.Fatalf("invalid URL sibling: %q", got)
	}
	if webPFormatSibling("", ".avif") != "" || webPFormatSibling("a.jpg", ".avif") != "" {
		t.Fatal("webp sibling guards")
	}
	if PrefersAVIF("image/avif;q=abc") {
		t.Fatal("bad q")
	}
	if !PrefersAVIF("image/avif;q=0.8") {
		t.Fatal("positive q")
	}
	if PrefersAVIF(" , image/webp") {
		t.Fatal("empty parts")
	}
}
