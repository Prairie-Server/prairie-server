package userdb

import (
	"database/sql"
	"testing"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

func TestSeriesPlaybackPreferenceRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	pref := userstore.SeriesPlaybackPreference{
		ProfileID:  "profile-1",
		SeriesID:   "series-1",
		Resolution: "1080p",
		HDR:        false,
		CodecVideo: "h264",
		UpdatedAt:  "2026-04-07T00:00:00Z",
	}
	if err := SetSeriesPlaybackPreference(db, pref); err != nil {
		t.Fatalf("SetSeriesPlaybackPreference: %v", err)
	}

	got, err := GetSeriesPlaybackPreference(db, "profile-1", "series-1")
	if err != nil {
		t.Fatalf("GetSeriesPlaybackPreference: %v", err)
	}
	if got == nil {
		t.Fatal("expected stored preference")
	}
	if got.Resolution != "1080p" || got.CodecVideo != "h264" || got.HDR {
		t.Fatalf("stored pref = %+v, want 1080p/h264/false", got)
	}

	if err := DeleteSeriesPlaybackPreference(db, "profile-1", "series-1"); err != nil {
		t.Fatalf("DeleteSeriesPlaybackPreference: %v", err)
	}

	got, err = GetSeriesPlaybackPreference(db, "profile-1", "series-1")
	if err != nil {
		t.Fatalf("GetSeriesPlaybackPreference(after delete): %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}
