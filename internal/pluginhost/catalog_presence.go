package pluginhost

import (
	"context"
	"fmt"
)

// PresenceLookupFunc is the minimal slice of catalog functionality
// CatalogPresence needs from silo. It returns the same record type the
// adapter exports.
type PresenceLookupFunc func(ctx context.Context, mediaType string, tmdbIDs []string) ([]LibraryPresenceRecord, error)

// CatalogPresence implements CatalogPresenceLookup by delegating to a
// caller-supplied function. Production wiring (in cmd/prairie/main.go)
// passes a closure over *catalog.MetadataRepository's TMDB-id lookup.
type CatalogPresence struct {
	lookup PresenceLookupFunc
}

func NewCatalogPresence(lookup PresenceLookupFunc) *CatalogPresence {
	return &CatalogPresence{lookup: lookup}
}

// LookupByExternalIDs satisfies CatalogPresenceLookup. v1 supports
// provider="tmdb" only.
func (c *CatalogPresence) LookupByExternalIDs(ctx context.Context, provider, mediaType string, ids []string) ([]LibraryPresenceRecord, error) {
	if provider != "tmdb" {
		return nil, nil
	}
	if c.lookup == nil {
		return nil, nil
	}
	// Prairie's media types differ from the SDK's: SDK uses "tv",
	// silo uses "series". Map at the boundary.
	internalType := mediaType
	if mediaType == "tv" {
		internalType = "series"
	}
	rows, err := c.lookup(ctx, internalType, ids)
	if err != nil {
		return nil, fmt.Errorf("catalog lookup: %w", err)
	}
	return rows, nil
}
