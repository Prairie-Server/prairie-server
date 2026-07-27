package jellycompat

import (
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/catalog"
)

func TestCompatChapters(t *testing.T) {
	t.Parallel()
	addedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	items := compatChapters([]catalog.VersionChapter{
		{
			Title:              "Cold Open",
			StartSeconds:       0,
			EndSeconds:         60,
			ThumbnailURL:       "https://example.test/chapter.webp",
			ThumbnailThumbhash: "thumbhash-1",
		},
		{
			Title:        "No Image",
			StartSeconds: 60,
			EndSeconds:   120,
		},
	}, addedAt)

	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if got := items[0]["Name"]; got != "Cold Open" {
		t.Fatalf("items[0][Name] = %v, want Cold Open", got)
	}
	if got := items[0]["ImagePath"]; got != "https://example.test/chapter.webp" {
		t.Fatalf("items[0][ImagePath] = %v", got)
	}
	if got := items[0]["ImageTag"]; got != "thumbhash-1" {
		t.Fatalf("items[0][ImageTag] = %v", got)
	}
	if _, ok := items[1]["ImagePath"]; ok {
		t.Fatalf("items[1] should omit ImagePath: %#v", items[1])
	}

	wantDate := "2025-06-01T12:00:00Z"
	for i, item := range items {
		if got := item["ImageDateModified"]; got != wantDate {
			t.Fatalf("items[%d][ImageDateModified] = %v, want %s", i, got, wantDate)
		}
	}
}

func TestCompatChaptersZeroAddedAt(t *testing.T) {
	t.Parallel()
	items := compatChapters([]catalog.VersionChapter{
		{Title: "Chapter 1", StartSeconds: 0, EndSeconds: 30},
	}, time.Time{})

	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	got, ok := items[0]["ImageDateModified"]
	if !ok {
		t.Fatal("ImageDateModified missing on zero addedAt")
	}
	if got == "" {
		t.Fatal("ImageDateModified must not be empty string on zero addedAt")
	}
	if got != "1970-01-01T00:00:00Z" {
		t.Fatalf("ImageDateModified = %v, want 1970-01-01T00:00:00Z", got)
	}
}
