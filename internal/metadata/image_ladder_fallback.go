package metadata

import (
	"context"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/prairie-server/prairie-server/internal/artworkkey"
	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/imagesize"
)

// ArtworkAvailability is durable knowledge about one immutable revision.
// Verified distinguishes a completed delivery check (including no usable keys)
// from publication that has not yet been checked through external delivery.
type ArtworkAvailability struct {
	Published   []string
	Deliverable []string
	External    bool
	Verified    bool
}

type ArtworkAvailabilityReader interface {
	ArtworkAvailability(context.Context, []string) (map[string]ArtworkAvailability, error)
}

// These types gained their widest rung in the current ladder version. Legacy
// artwork without a manifest can safely use only an established lower rung.
//
// Prairie keeps its TV-tuned ladder (artworkkey.VariantWidths): posters top
// out at w500, stills at w500 and logos at w500, all rungs every cached item
// has always carried. Upstream's w780/w1280 rungs do not exist here, so no
// widest rung is new, and stepping an unverified w500 down to w300 would only
// serve every default-size poster smaller than it needs to be. Prairie's own
// added rung (the w200 TV poster) sits at the bottom of the ladder, which this
// step-down never concerns.
var ladderTypesWithAddedRung = map[string]bool{}

// keyVariant extracts the variant name from a cached artwork key, e.g.
// "tmdb/movies/550/poster/w780.abc123.webp" -> "w780".
func keyVariant(key string) string {
	name := path.Base(strings.TrimSpace(key))
	name = strings.TrimSuffix(name, path.Ext(name))
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		name = name[:dot]
	}
	return name
}

// variantKey rebuilds a cached artwork key at a different rung, preserving the
// revision and extension.
func variantKey(key, variant string) string {
	dir := strings.TrimSuffix(artworkkey.Directory(key), "/")
	if dir == "" {
		return key
	}
	original := artworkkey.Original(dir, artworkkey.Revision(key), path.Ext(key))
	return artworkkey.Variant(original, variant)
}

// resolvePublishedLadderKeys performs one catalog lookup and local selection.
// Neither cold caches nor missing manifests cause storage or delivery probes.
// Without a reader (local storage, direct S3) every key resolves to itself:
// publication is atomic there, and a key that is nevertheless missing gets a
// 404 that enqueues repair rather than a silently demoted rung.
func resolvePublishedLadderKeys(ctx context.Context, reader ArtworkAvailabilityReader, entries []resolveEntry) map[string]string {
	if reader == nil {
		resolved := make(map[string]string, len(entries))
		for _, entry := range entries {
			resolved[entry.originalPath] = entry.originalPath
		}
		return resolved
	}
	originals := make([]string, 0, len(entries))
	for _, entry := range entries {
		if catalog.ImageTypeFromCachedPath(entry.originalPath) != "" {
			originals = append(originals, variantKey(entry.originalPath, artworkkey.OriginalVariant))
		}
	}
	var states map[string]ArtworkAvailability
	if len(originals) > 0 {
		var err error
		states, err = reader.ArtworkAvailability(ctx, originals)
		if err != nil {
			slog.WarnContext(ctx, "artwork publication lookup failed; using established variants", "error", err)
		}
	}
	resolved := make(map[string]string, len(entries))
	for _, entry := range entries {
		key := entry.originalPath
		state, known := states[variantKey(key, artworkkey.OriginalVariant)]
		resolved[key] = selectPublishedVariant(key, state, known)
	}
	return resolved
}

func selectPublishedVariant(key string, state ArtworkAvailability, known bool) string {
	imageType := catalog.ImageTypeFromCachedPath(key)
	if imageType == "" {
		return key
	}
	if !known || (state.External && !state.Verified) {
		// An unverified external path is not proof the newly added size works.
		if ladderTypesWithAddedRung[imageType] && keyVariant(key) == imagesize.Variant(imageType, imagesize.Large) {
			if lower, ok := imagesize.NextLower(imageType, keyVariant(key)); ok {
				key = variantKey(key, lower)
			}
		}
		if !known {
			return key
		}
	}
	keys := state.Published
	if state.External && state.Verified {
		keys = state.Deliverable
	}
	candidate := key
	if keyVariant(key) == artworkkey.OriginalVariant && !slices.Contains(keys, key) {
		candidate = variantKey(key, imagesize.Variant(imageType, imagesize.Large))
	}
	for {
		if slices.Contains(keys, candidate) {
			return candidate
		}
		lower, ok := imagesize.NextLower(imageType, keyVariant(candidate))
		if !ok {
			break
		}
		candidate = variantKey(key, lower)
	}
	original := variantKey(key, artworkkey.OriginalVariant)
	if slices.Contains(keys, original) {
		return original
	}
	// A known empty manifest or failed delivery is a placeholder, not a guess.
	return ""
}

func minURLExpiry(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
