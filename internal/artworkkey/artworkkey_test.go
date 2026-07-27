package artworkkey

import "testing"

func TestImageTypeFromPath(t *testing.T) {
	if got := ImageTypeFromPath("tmdb/movies/550/poster/original.abc.webp"); got != "poster" {
		t.Fatalf("ImageTypeFromPath() = %q, want poster", got)
	}
	if got := ImageTypeFromPath("https://example.com/poster/original.webp"); got != "" {
		t.Fatalf("ImageTypeFromPath(url) = %q, want empty", got)
	}
}

func TestLegacyArtworkKeysRemainSupported(t *testing.T) {
	original := "tmdb/movies/550/poster/original.webp"
	if got := Variant(original, "w300"); got != "tmdb/movies/550/poster/w300.webp" {
		t.Fatalf("Variant() = %q", got)
	}
	if got := Revision(original); got != "" {
		t.Fatalf("Revision() = %q, want empty", got)
	}
}

func TestVariantOnlyRewritesOriginalFilename(t *testing.T) {
	original := "tmdb/movies/original.segment/550/poster/original.abc123.webp"
	want := "tmdb/movies/original.segment/550/poster/w500.abc123.webp"
	if got := Variant(original, "w500"); got != want {
		t.Fatalf("Variant() = %q, want %q", got, want)
	}
}

func TestFormatSiblingAndObjectKeysIncludeAVIFAndPNG(t *testing.T) {
	original := "tmdb/movies/550/poster/original.abc123.webp"
	if got := WebPAVIFSibling(original); got != "tmdb/movies/550/poster/original.abc123.avif" {
		t.Fatalf("WebPAVIFSibling() = %q", got)
	}
	if got := WebPPNGSibling(original); got != "tmdb/movies/550/poster/original.abc123.png" {
		t.Fatalf("WebPPNGSibling() = %q", got)
	}
	url := "https://cdn.example/tmdb/movies/550/poster/original.abc123.webp?token=1"
	wantURL := "https://cdn.example/tmdb/movies/550/poster/original.abc123.avif?token=1"
	if got := WebPAVIFSibling(url); got != wantURL {
		t.Fatalf("WebPAVIFSibling(url) = %q, want %q", got, wantURL)
	}
	wantPNGURL := "https://cdn.example/tmdb/movies/550/poster/original.abc123.png?token=1"
	if got := WebPPNGSibling(url); got != wantPNGURL {
		t.Fatalf("WebPPNGSibling(url) = %q, want %q", got, wantPNGURL)
	}
	if got := WebPAVIFSibling("tmdb/movies/550/poster/original.jpg"); got != "" {
		t.Fatalf("WebPAVIFSibling(jpeg) = %q, want empty", got)
	}
	if got := WebPPNGSibling("tmdb/movies/550/poster/original.jpg"); got != "" {
		t.Fatalf("WebPPNGSibling(jpeg) = %q, want empty", got)
	}
	avif, png := ObjectFormatSiblings("tmdb/movies/550/poster/original.abc123.webp")
	if avif != "tmdb/movies/550/poster/original.abc123.avif" || png != "tmdb/movies/550/poster/original.abc123.png" {
		t.Fatalf("ObjectFormatSiblings(webp) = %q, %q", avif, png)
	}
	avif, png = ObjectFormatSiblings("plugin://tmdb/poster/x.webp")
	if avif != "" || png != "" {
		t.Fatalf("ObjectFormatSiblings(plugin) = %q, %q, want empty", avif, png)
	}
	avif, png = ObjectFormatSiblings(url)
	if avif != "" || png != "" {
		t.Fatalf("ObjectFormatSiblings(https) = %q, %q, want empty", avif, png)
	}
	keys := ObjectKeys(original, "poster")
	want := map[string]bool{
		"tmdb/movies/550/poster/original.abc123.webp": true,
		"tmdb/movies/550/poster/original.abc123.avif": true,
		"tmdb/movies/550/poster/original.abc123.png":  true,
		"tmdb/movies/550/poster/w500.abc123.webp":     true,
		"tmdb/movies/550/poster/w500.abc123.avif":     true,
		"tmdb/movies/550/poster/w500.abc123.png":      true,
		"tmdb/movies/550/poster/w300.abc123.webp":     true,
		"tmdb/movies/550/poster/w300.abc123.avif":     true,
		"tmdb/movies/550/poster/w300.abc123.png":      true,
	}
	if len(keys) != len(want) {
		t.Fatalf("ObjectKeys len = %d, want %d (%v)", len(keys), len(want), keys)
	}
	for _, key := range keys {
		if !want[key] {
			t.Fatalf("unexpected key %q", key)
		}
	}
}

func TestParseImageFormats(t *testing.T) {
	if got := ParseImageFormats("avif,webp,png"); len(got) != 3 || got[0] != FormatAVIF {
		t.Fatalf("ParseImageFormats() = %v", got)
	}
	if got := ParseImageFormats(" WEBP , avif , webp "); len(got) != 2 || got[0] != FormatWebP || got[1] != FormatAVIF {
		t.Fatalf("ParseImageFormats(dedupe) = %v", got)
	}
	if got := ParseImageFormats("jpeg,foo"); len(got) != 0 {
		t.Fatalf("ParseImageFormats(unknown) = %v, want empty", got)
	}
}

func TestSelectRasterURL(t *testing.T) {
	canonical := "https://cdn/poster.webp"
	avif := "https://cdn/poster.avif"
	png := "https://cdn/poster.png"

	if got := SelectRasterURL(canonical, avif, png, []string{FormatAVIF, FormatWebP, FormatPNG}); got != avif {
		t.Fatalf("SelectRasterURL(avif first) = %q", got)
	}
	if got := SelectRasterURL(canonical, "", png, []string{FormatAVIF, FormatWebP, FormatPNG}); got != canonical {
		t.Fatalf("SelectRasterURL(missing avif) = %q", got)
	}
	if got := SelectRasterURL(canonical, avif, png, []string{FormatWebP, FormatPNG}); got != canonical {
		t.Fatalf("SelectRasterURL(webp first) = %q", got)
	}
	if got := SelectRasterURL("", "", png, nil); got != png {
		t.Fatalf("SelectRasterURL(default png) = %q", got)
	}
	if got := SelectRasterURL("", avif, "", nil); got != avif {
		t.Fatalf("SelectRasterURL(default avif) = %q", got)
	}
	if got := SelectRasterURL(canonical, avif, png, nil); got != canonical {
		t.Fatalf("SelectRasterURL(default webp) = %q", got)
	}
	if got := SelectRasterURL("", "", "", []string{FormatAVIF}); got != "" {
		t.Fatalf("SelectRasterURL(empty) = %q", got)
	}
	if got := SelectRasterURL("  ", avif, png, []string{FormatWebP, FormatAVIF}); got != avif {
		t.Fatalf("SelectRasterURL(trim blank webp) = %q", got)
	}
	if got := SelectRasterURL("", "", png, []string{FormatAVIF, FormatWebP}); got != png {
		t.Fatalf("SelectRasterURL(prefer miss → png) = %q", got)
	}
}

func TestImageFormatsFromRequest(t *testing.T) {
	if got := ImageFormatsFromRequest("webp,png", ""); len(got) != 2 || got[0] != FormatWebP {
		t.Fatalf("ImageFormatsFromRequest(header) = %v", got)
	}
	if got := ImageFormatsFromRequest("", "image/avif,image/webp"); len(got) != 3 || got[0] != FormatAVIF {
		t.Fatalf("ImageFormatsFromRequest(accept) = %v", got)
	}
	if got := ImageFormatsFromRequest("", "image/webp"); got != nil {
		t.Fatalf("ImageFormatsFromRequest(no avif) = %v, want nil", got)
	}
}

func TestPrefersAVIF(t *testing.T) {
	cases := []struct {
		accept string
		want   bool
	}{
		{"", false},
		{"image/webp", false},
		{"image/avif,image/webp", true},
		{"image/avif;q=0,image/webp", false},
		{"image/webp,image/*,*/*;q=0.8", false},
		{"text/html", false},
	}
	for _, tc := range cases {
		if got := PrefersAVIF(tc.accept); got != tc.want {
			t.Fatalf("PrefersAVIF(%q) = %v, want %v", tc.accept, got, tc.want)
		}
	}
}
