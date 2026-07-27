package handlers

import (
	"github.com/prairie-server/prairie-server/internal/artworkkey"
	"github.com/prairie-server/prairie-server/internal/catalog"
)

// artworkFormats is the additive AVIF/PNG sibling set for a canonical artwork URL.
// Canonical stays on poster_url / backdrop_url (usually .webp); clients prefer
// the signed AVIF sibling when present, then WebP, then PNG.
type artworkFormats struct {
	URL     string
	AVIFURL string
	PNGURL  string
}

// appendArtworkFormatPaths adds a canonical path plus any bare-object AVIF/PNG
// siblings to paths for batch presigning.
func appendArtworkFormatPaths(paths []string, seen map[string]struct{}, path string) []string {
	if path == "" || path == "-" {
		return paths
	}
	paths = appendUniquePath(paths, seen, path)
	avif, png := artworkkey.ObjectFormatSiblings(path)
	paths = appendUniquePath(paths, seen, avif)
	paths = appendUniquePath(paths, seen, png)
	return paths
}

func appendUniquePath(paths []string, seen map[string]struct{}, path string) []string {
	if path == "" {
		return paths
	}
	if _, ok := seen[path]; ok {
		return paths
	}
	seen[path] = struct{}{}
	return append(paths, path)
}

// artworkFormatsFromResolved picks canonical + signed sibling URLs from a
// PresignURLsWithExpiry result. Missing sibling objects still get a signed URL
// that 404s; clients fall through via ArtworkImage onError (AVIF→WebP→PNG).
// New caches omit PNG; the PNG URL remains for legacy objects and older clients.
func artworkFormatsFromResolved(resolved map[string]catalog.ResolvedImageURL, path string) artworkFormats {
	if path == "" || path == "-" {
		return artworkFormats{}
	}
	out := artworkFormats{URL: resolved[path].URL}
	avif, png := artworkkey.ObjectFormatSiblings(path)
	if avif != "" {
		out.AVIFURL = resolved[avif].URL
	}
	if png != "" {
		out.PNGURL = resolved[png].URL
	}
	return out
}
