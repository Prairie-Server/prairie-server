// Package artworkkey owns the object-key naming contract for cached artwork.
// Legacy names such as original.webp and revisioned names such as
// original.<revision>.webp are both supported.
package artworkkey

import (
	"net/url"
	"path"
	"strconv"
	"strings"
)

const OriginalVariant = "original"

// Build returns an object key for a variant under basePath.
func Build(basePath, variant, revision, ext string) string {
	basePath = strings.TrimRight(strings.TrimSpace(basePath), "/")
	variant = strings.TrimSpace(variant)
	revision = strings.TrimSpace(revision)
	if basePath == "" || variant == "" {
		return ""
	}
	if ext == "" {
		ext = ".webp"
	} else if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if revision == "" {
		return basePath + "/" + variant + ext
	}
	return basePath + "/" + variant + "." + revision + ext
}

// Original returns the original-variant key under basePath.
func Original(basePath, revision, ext string) string {
	return Build(basePath, OriginalVariant, revision, ext)
}

// Variant rewrites an original key to another variant while retaining any
// revision and extension. Unrecognized paths pass through unchanged.
func Variant(originalPath, variant string) string {
	if originalPath == "" || variant == "" || variant == OriginalVariant {
		return originalPath
	}
	dir := path.Dir(originalPath)
	base := path.Base(originalPath)
	if dir == "." || !strings.HasPrefix(base, OriginalVariant+".") {
		return originalPath
	}
	return strings.TrimRight(dir, "/") + "/" + variant + strings.TrimPrefix(base, OriginalVariant)
}

// Directory returns the image-type prefix containing every revision and
// variant for an artwork key, including a trailing slash.
func Directory(objectPath string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" || strings.Contains(objectPath, "://") {
		return ""
	}
	dir := path.Dir(objectPath)
	if dir == "." || dir == "/" {
		return ""
	}
	return strings.TrimRight(dir, "/") + "/"
}

// Revision extracts the content revision from a revisioned key. Legacy keys
// return an empty string.
func Revision(objectPath string) string {
	name := path.Base(strings.TrimSpace(objectPath))
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	firstDot := strings.IndexByte(stem, '.')
	if firstDot < 0 || firstDot == len(stem)-1 {
		return ""
	}
	return stem[firstDot+1:]
}

// VariantWidths returns the resize widths generated for an artwork type. This
// is the single source of truth for the variant ladder: image generation,
// object-key expansion, and garbage collection all derive from it.
func VariantWidths(imageType string) []int {
	switch strings.ToLower(strings.TrimSpace(imageType)) {
	case "backdrop":
		return []int{1920, 1280, 300}
	case "logo":
		return []int{500}
	default: // poster, still, profile
		return []int{500, 300}
	}
}

// VariantNames returns the cached variants generated for an artwork type.
func VariantNames(imageType string) []string {
	widths := VariantWidths(imageType)
	names := make([]string, 0, len(widths)+1)
	names = append(names, OriginalVariant)
	for _, width := range widths {
		names = append(names, "w"+strconv.Itoa(width))
	}
	return names
}

// ObjectKeys expands an original key to every expected key for its image type,
// including AVIF and PNG siblings when the canonical key is WebP.
func ObjectKeys(originalPath, imageType string) []string {
	if originalPath == "" || strings.Contains(originalPath, "://") {
		return nil
	}
	names := VariantNames(imageType)
	keys := make([]string, 0, len(names)*3)
	for _, name := range names {
		webpKey := Variant(originalPath, name)
		keys = append(keys, webpKey)
		if avifKey := WebPAVIFSibling(webpKey); avifKey != "" {
			keys = append(keys, avifKey)
		}
		if pngKey := WebPPNGSibling(webpKey); pngKey != "" {
			keys = append(keys, pngKey)
		}
	}
	return keys
}

// FormatSibling returns the same object key or URL with a different extension.
// For http(s) URLs only the path suffix is rewritten; query/fragment stay put.
func FormatSibling(objectPath, ext string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" {
		return objectPath
	}
	if ext == "" {
		return objectPath
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if strings.Contains(objectPath, "://") {
		u, err := url.Parse(objectPath)
		if err != nil || u.Path == "" {
			return objectPath
		}
		cur := path.Ext(u.Path)
		if cur == "" {
			return objectPath
		}
		u.Path = strings.TrimSuffix(u.Path, cur) + ext
		return u.String()
	}
	cur := path.Ext(objectPath)
	if cur == "" {
		return objectPath
	}
	return strings.TrimSuffix(objectPath, cur) + ext
}

// WebPAVIFSibling returns the AVIF sibling path/URL for a canonical WebP key.
// Non-WebP inputs return empty so callers do not invent AVIF objects.
func WebPAVIFSibling(objectPath string) string {
	return webPFormatSibling(objectPath, ".avif")
}

// WebPPNGSibling returns the PNG sibling path/URL for a canonical WebP key.
// Non-WebP inputs return empty so callers do not invent PNG objects.
func WebPPNGSibling(objectPath string) string {
	return webPFormatSibling(objectPath, ".png")
}

// ObjectFormatSiblings returns AVIF and PNG sibling object keys for a bare
// cached WebP path. Empty for http(s)/plugin URLs and non-WebP keys — those
// must not be rewritten before signing (signatures cover the object key).
func ObjectFormatSiblings(objectPath string) (avif, png string) {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" || strings.Contains(objectPath, "://") {
		return "", ""
	}
	return WebPAVIFSibling(objectPath), WebPPNGSibling(objectPath)
}

func webPFormatSibling(objectPath, ext string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" {
		return ""
	}
	cur := path.Ext(objectPath)
	if strings.Contains(objectPath, "://") {
		u, err := url.Parse(objectPath)
		if err != nil {
			return ""
		}
		cur = path.Ext(u.Path)
	}
	if !strings.EqualFold(cur, ".webp") {
		return ""
	}
	return FormatSibling(objectPath, ext)
}

const (
	FormatAVIF = "avif"
	FormatWebP = "webp"
	FormatPNG  = "png"
)

// ParseImageFormats parses X-Prairie-Image-Formats (or any comma-separated
// raster preference list) into normalized tokens in preference order.
// Unknown tokens are dropped; duplicates are removed.
func ParseImageFormats(header string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, part := range strings.Split(header, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token != FormatAVIF && token != FormatWebP && token != FormatPNG {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

// ImageFormatsFromRequest returns the client's raster preference list from
// X-Prairie-Image-Formats, falling back to Accept when the explicit header is
// absent.
func ImageFormatsFromRequest(imageFormatsHeader, acceptHeader string) []string {
	if parsed := ParseImageFormats(imageFormatsHeader); len(parsed) > 0 {
		return parsed
	}
	if PrefersAVIF(acceptHeader) {
		return []string{FormatAVIF, FormatWebP, FormatPNG}
	}
	return nil
}

// SelectRasterURL picks the first available URL from canonical WebP, AVIF, and
// PNG siblings using the client's ordered format preference. When preferences
// are empty, canonical WebP is returned.
func SelectRasterURL(canonical, avif, png string, preferred []string) string {
	byFormat := map[string]string{
		FormatWebP: canonical,
		FormatAVIF: avif,
		FormatPNG:  png,
	}
	if len(preferred) == 0 {
		if canonical != "" {
			return canonical
		}
		if avif != "" {
			return avif
		}
		return png
	}
	for _, format := range preferred {
		if url := strings.TrimSpace(byFormat[format]); url != "" {
			return url
		}
	}
	if canonical != "" {
		return canonical
	}
	if avif != "" {
		return avif
	}
	return png
}

// PrefersAVIF reports whether an Accept header explicitly includes image/avif
// with a positive q-value. Wildcards alone do not count — many clients send
// image/* without supporting AVIF.
func PrefersAVIF(accept string) bool {
	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		media, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(media), "image/avif") {
			continue
		}
		return acceptQuality(params) > 0
	}
	return false
}

func acceptQuality(params string) float64 {
	q := 1.0
	for _, param := range strings.Split(params, ";") {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}
		key, val, ok := strings.Cut(param, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return 0
		}
		q = parsed
	}
	return q
}
