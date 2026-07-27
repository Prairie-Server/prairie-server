package handlers

import (
	"context"
	"time"

	"github.com/prairie-server/prairie-server/internal/livetv"
)

// LiveTVHistoryRecorder writes finished Live TV views into the same admin
// playback history table as VOD sessions. Live TV has no media file, so rows
// carry media_file_id 0 and a channel-keyed media_item_id
// (livetv.HistoryMediaItemID) that the history query resolves to a channel name.
type LiveTVHistoryRecorder struct {
	store PlaybackAdminStore
}

// NewLiveTVHistoryRecorder adapts the admin playback store for Live TV.
// Returns nil when no store is configured, so callers can wire unconditionally.
func NewLiveTVHistoryRecorder(store PlaybackAdminStore) *LiveTVHistoryRecorder {
	if store == nil {
		return nil
	}
	return &LiveTVHistoryRecorder{store: store}
}

func (r *LiveTVHistoryRecorder) RecordLiveSession(ctx context.Context, entry livetv.SessionHistoryEntry) error {
	if r == nil || r.store == nil {
		return nil
	}
	// Anonymous/unauthenticated sessions have no user to attribute the view to.
	if entry.UserID == 0 {
		return nil
	}
	playMethod := entry.Transport
	if playMethod == "" {
		playMethod = playbackModeDirect
	}
	return r.store.RecordHistory(ctx, AdminPlaybackHistoryEntry{
		SessionID:      entry.SessionID,
		UserID:         entry.UserID,
		ProfileID:      entry.ProfileID,
		MediaItemID:    livetv.HistoryMediaItemID(entry.ChannelID),
		MediaFileID:    0,
		PlayMethod:     playMethod,
		StartedAt:      entry.StartedAt.UTC().Format(time.RFC3339Nano),
		EndedAt:        entry.EndedAt.UTC().Format(time.RFC3339Nano),
		WatchedSeconds: entry.WatchedSeconds,
	})
}
