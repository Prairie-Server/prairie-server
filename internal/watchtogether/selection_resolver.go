package watchtogether

import (
	"context"
	"strings"

	"github.com/prairie-server/prairie-server/internal/access"
	"github.com/prairie-server/prairie-server/internal/catalog"
)

type catalogWatchDetailLookup interface {
	GetWatchDetail(ctx context.Context, contentID string, filter catalog.AccessFilter) (*catalog.WatchDetail, error)
}

type CatalogSelectionResolver struct {
	details catalogWatchDetailLookup
}

func NewCatalogSelectionResolver(details catalogWatchDetailLookup) *CatalogSelectionResolver {
	return &CatalogSelectionResolver{details: details}
}

func (r *CatalogSelectionResolver) ResolveSelection(
	ctx context.Context,
	userID int,
	profileID string,
	input SelectItemInput,
) (*ResolvedSelection, error) {
	if r == nil || r.details == nil {
		return nil, ErrInvalidSelection
	}

	filter := catalog.AccessFilter{
		UserID:         userID,
		ProfileID:      profileID,
		SelectedFileID: 0,
	}
	if scope, ok := access.GetScope(ctx); ok {
		filter.AllowedLibraryIDs = scope.AllowedLibraryIDs
		filter.DisabledLibraryIDs = scope.DisabledLibraryIDs
		filter.MaxContentRating = scope.MaxContentRating
		filter.MaxPlaybackQuality = scope.MaxPlaybackQuality
	}
	if input.LibraryID != nil && *input.LibraryID > 0 {
		filter.PresentationLibraryID = input.LibraryID
	}
	if input.FileID != nil && *input.FileID > 0 {
		filter.SelectedFileID = *input.FileID
	}

	detail, err := r.details.GetWatchDetail(ctx, strings.TrimSpace(input.ContentID), filter)
	if err != nil {
		return nil, err
	}
	if detail == nil || (detail.Type != "movie" && detail.Type != "episode") || len(detail.Versions) == 0 {
		return nil, ErrInvalidSelection
	}

	resolved := &ResolvedSelection{
		ContentID: detail.ContentID,
		LibraryID: input.LibraryID,
	}
	if input.FileID != nil {
		for _, version := range detail.Versions {
			if version.FileID == *input.FileID {
				resolved.FileID = input.FileID
				return resolved, nil
			}
		}
		return nil, ErrInvalidSelection
	}

	resolvedFileID := detail.Versions[0].FileID
	resolved.FileID = &resolvedFileID
	return resolved, nil
}
