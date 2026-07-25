package userdb

import (
	"database/sql"
	"testing"

	"github.com/prairie-server/prairie-server/internal/userstore"
)

func TestAudioPreference_TrackSignatureRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	if err := SetAudioPreference(db, userstore.AudioPreference{
		ProfileID:       "profile-1",
		SeriesID:        "series-1",
		AudioTrackIndex: 2,
		AudioLanguage:   "eng",
		TrackSignature: &userstore.AudioTrackSignature{
			Language: "eng",
			Title:    "English 5.1",
			Codec:    "flac",
			Layout:   "5.1",
			Channels: 6,
		},
		UpdatedAt: "2026-04-08T00:00:00Z",
	}); err != nil {
		t.Fatalf("SetAudioPreference: %v", err)
	}

	pref, err := GetAudioPreference(db, "profile-1", "series-1")
	if err != nil {
		t.Fatalf("GetAudioPreference: %v", err)
	}
	if pref == nil || pref.TrackSignature == nil {
		t.Fatal("expected audio preference signature to round-trip")
	}
	if pref.TrackSignature.Layout != "5.1" {
		t.Fatalf("TrackSignature.Layout = %q, want 5.1", pref.TrackSignature.Layout)
	}
}

func TestSubtitlePreference_TrackSignatureRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	if err := SetSubtitlePreference(db, userstore.SubtitlePreference{
		ProfileID:        "profile-1",
		SeriesID:         "series-1",
		SubtitleLanguage: "en",
		SubtitleMode:     "always",
		TrackSignature: &userstore.SubtitleTrackSignature{
			Source:          "external",
			Language:        "en",
			Codec:           "srt",
			Label:           "English SDH",
			HearingImpaired: true,
		},
		UpdatedAt: "2026-04-08T00:00:00Z",
	}); err != nil {
		t.Fatalf("SetSubtitlePreference: %v", err)
	}

	pref, err := GetSubtitlePreference(db, "profile-1", "series-1")
	if err != nil {
		t.Fatalf("GetSubtitlePreference: %v", err)
	}
	if pref == nil || pref.TrackSignature == nil {
		t.Fatal("expected subtitle preference signature to round-trip")
	}
	if pref.TrackSignature.Label != "English SDH" {
		t.Fatalf("TrackSignature.Label = %q, want English SDH", pref.TrackSignature.Label)
	}
	if !pref.TrackSignature.HearingImpaired {
		t.Fatal("expected subtitle track signature hearing impaired flag")
	}
}
