package branding

import (
	"fmt"
	"net/http"
	"path"
	"slices"

	"github.com/prairie-server/prairie-server/internal/imageutil"
)

// assetSpec describes how one branding image kind is validated, processed, and
// stored.
type assetSpec struct {
	kind       AssetKind
	settingKey string // server_settings key holding the current content ref
	s3Prefix   string // S3 key prefix; object key is "<prefix>/<ref>"
	maxBytes   int64
	// process validates the declared content type and returns the bytes to
	// store, optional AVIF/PNG sibling bytes, the content type to serve the
	// canonical object with, and the file extension used in the content ref.
	process func(data []byte, declaredType string) (out, avif, png []byte, serveType, ext string, err error)
}

// assetSpecs is the registry of all branding asset kinds. Adding a kind here is
// the only change needed to support a new uploadable asset.
var assetSpecs = map[AssetKind]assetSpec{
	KindWordmark: {
		kind:       KindWordmark,
		settingKey: "branding.wordmark_ref",
		s3Prefix:   "branding/wordmark",
		maxBytes:   8 << 20,
		process:    processWebP(imageutil.GenerateVariants, 640),
	},
	KindWordmarkLight: {
		kind:       KindWordmarkLight,
		settingKey: "branding.wordmark_light_ref",
		s3Prefix:   "branding/wordmark-light",
		maxBytes:   8 << 20,
		process:    processWebP(imageutil.GenerateVariants, 640),
	},
	KindMark: {
		kind:       KindMark,
		settingKey: "branding.mark_ref",
		s3Prefix:   "branding/mark",
		maxBytes:   8 << 20,
		process:    processWebP(imageutil.GenerateSquareVariants, 512),
	},
	KindMarkLight: {
		kind:       KindMarkLight,
		settingKey: "branding.mark_light_ref",
		s3Prefix:   "branding/mark-light",
		maxBytes:   8 << 20,
		process:    processWebP(imageutil.GenerateSquareVariants, 512),
	},
	KindFavicon: {
		kind:       KindFavicon,
		settingKey: "branding.favicon_ref",
		s3Prefix:   "branding/favicon",
		maxBytes:   1 << 20,
		process:    processFaviconPassthrough,
	},
	KindLoginBg: {
		kind:       KindLoginBg,
		settingKey: "branding.login_bg_ref",
		s3Prefix:   "branding/login-bg",
		maxBytes:   12 << 20,
		process:    processWebP(imageutil.GenerateVariants, 2560),
	},
}

const (
	extWebP  = ".webp"
	mimeWebP = "image/webp"
)

// imageUploadTypes is the accepted set for WebP-converted kinds.
// AVIF uploads are not accepted here: the WASI decoder does not decode AVIF,
// and re-encoding an AVIF source would need a separate decode path.
var imageUploadTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	mimeWebP:     true,
}

// imageVariantFunc matches imageutil.GenerateVariants / GenerateSquareVariants.
type imageVariantFunc func(data []byte, sizes []int) (*imageutil.VariantResult, error)

// processWebP re-encodes any accepted image to a size-capped WebP (+ AVIF
// siblings) using the given variant generator. The aspect ratio is preserved by
// the generator; narrower images are not upscaled.
func processWebP(generate imageVariantFunc, size int) func([]byte, string) ([]byte, []byte, []byte, string, string, error) {
	return func(data []byte, declaredType string) ([]byte, []byte, []byte, string, string, error) {
		if !imageUploadTypes[declaredType] {
			return nil, nil, nil, "", "", ErrUnsupportedImage
		}
		res, err := generate(data, []int{size})
		if err != nil {
			return nil, nil, nil, "", "", ErrUnsupportedImage
		}
		key := fmt.Sprintf("w%d", size)
		webp, avif, png := pickVariantTriple(res, key)
		return webp, avif, png, mimeWebP, extWebP, nil
	}
}

// processFaviconPassthrough stores the favicon unchanged (no WebP re-encode) so
// that .ico/.png keep working in browsers that don't render WebP favicons.
func processFaviconPassthrough(data []byte, declaredType string) ([]byte, []byte, []byte, string, string, error) {
	switch declaredType {
	case "image/png":
		return data, nil, nil, "image/png", ".png", nil
	case mimeWebP:
		return data, nil, nil, mimeWebP, extWebP, nil
	case "image/avif":
		return data, nil, nil, "image/avif", ".avif", nil
	case "image/x-icon", "image/vnd.microsoft.icon":
		return data, nil, nil, "image/x-icon", ".ico", nil
	case "image/svg+xml":
		return data, nil, nil, "image/svg+xml", ".svg", nil
	default:
		return nil, nil, nil, "", "", ErrUnsupportedImage
	}
}

// pickVariantTriple returns WebP, AVIF, and PNG payloads from the *same*
// variant, falling back to "original" for all three so Accept negotiation
// never pairs mismatched dimensions.
func pickVariantTriple(res *imageutil.VariantResult, key string) (webp, avif, png []byte) {
	var origWebP []byte
	for _, v := range res.Variants {
		if v.Key == key {
			return v.Data, nil, nil
		}
		if v.Key == "original" {
			origWebP = v.Data
		}
	}
	return origWebP, nil, nil
}

// MaxUploadBytes returns the maximum accepted upload size for a kind, or 0 when
// the kind is unknown.
func MaxUploadBytes(kind AssetKind) int64 {
	if spec, ok := assetSpecs[kind]; ok {
		return spec.maxBytes
	}
	return 0
}

// CheckStoredAsset applies an upload's checks to an asset stored under key,
// as a restore writes one: key names a kind's object, data is within the
// kind's size limit, its bytes are the type its extension serves (an SVG
// favicon is not sniffed, as uploads do not sniff it), and the kind's
// processing accepts them and stores that extension. It wraps
// ErrUnsupportedImage, or ErrInvalidKind for a key no kind stores.
func CheckStoredAsset(key string, data []byte) error {
	prefix, ref := path.Split(key)
	var spec assetSpec
	found := false
	for _, s := range assetSpecs {
		if s.s3Prefix+"/" == prefix {
			spec, found = s, true
		}
	}
	if !found || ref == "" {
		return fmt.Errorf("%w: %s", ErrInvalidKind, key)
	}
	if int64(len(data)) > spec.maxBytes {
		return fmt.Errorf("%w: %d bytes is over the %d a %s may be", ErrUnsupportedImage, len(data), spec.maxBytes, spec.kind)
	}
	ext := path.Ext(ref)
	served := contentTypeForExt(ext)
	if served != "image/svg+xml" && http.DetectContentType(data) != served {
		return fmt.Errorf("%w: the file is not %s", ErrUnsupportedImage, served)
	}
	if _, _, _, _, stored, err := spec.process(data, served); err != nil || stored != ext {
		return fmt.Errorf("%w: a %s upload is not stored as %s", ErrUnsupportedImage, spec.kind, ext)
	}
	return nil
}

// contentTypeForExt maps a stored ref's extension to the Content-Type used when
// serving it.
func contentTypeForExt(ext string) string {
	switch ext {
	case extWebP:
		return mimeWebP
	case ".avif":
		return "image/avif"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// AssetSettingKeys lists the server_settings keys that hold a branding
// asset's content ref, sorted. Backups read them to find the asset objects.
func AssetSettingKeys() []string {
	keys := make([]string, 0, len(assetSpecs))
	for _, spec := range assetSpecs {
		keys = append(keys, spec.settingKey)
	}
	slices.Sort(keys)
	return keys
}

// AssetObjectKey is the blob key of the asset whose ref is stored under
// settingKey, or "" when the key holds no asset or the ref is empty.
func AssetObjectKey(settingKey, ref string) string {
	if ref == "" {
		return ""
	}
	for _, spec := range assetSpecs {
		if spec.settingKey == settingKey {
			return spec.s3Prefix + "/" + ref
		}
	}
	return ""
}
