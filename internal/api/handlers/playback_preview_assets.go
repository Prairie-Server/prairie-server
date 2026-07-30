package handlers

import (
	"context"
	"log/slog"

	"github.com/prairie-server/prairie-server/internal/models"
)

// queuePlaybackPreviewAssets prioritizes chapter-thumbnail and trickplay
// generation near the session start position. Shared by legacy and protocol-v3
// playback starts so seek previews do not wait on periodic backfill.
func (h *PlaybackHandler) queuePlaybackPreviewAssets(
	ctx context.Context,
	file *models.MediaFile,
	targetSeconds float64,
	source string,
) {
	if h == nil || file == nil || file.ID <= 0 {
		return
	}
	if h.ChapterThumbnailQueuer != nil {
		slog.InfoContext(ctx,
			"queueing chapter thumbnails", "component", "api",
			"source", source,
			"content_id", file.ContentID,
			"file_id", file.ID,
			"target_seconds", targetSeconds,
		)
		h.ChapterThumbnailQueuer.QueuePriorityFileAtPosition(ctx, file.ID, targetSeconds)
	}
	if h.TrickplayQueuer != nil {
		slog.InfoContext(ctx,
			"queueing trickplay", "component", "api",
			"source", source,
			"content_id", file.ContentID,
			"file_id", file.ID,
			"target_seconds", targetSeconds,
		)
		h.TrickplayQueuer.QueuePriorityFileAtPosition(ctx, file.ID, targetSeconds)
	}
}
