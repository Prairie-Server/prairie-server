package metadata

import (
	"reflect"
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func metadataReleaseDateField(t *testing.T, result *MetadataResult) reflect.Value {
	t.Helper()

	field := reflect.ValueOf(result).Elem().FieldByName("ReleaseDate")
	if !field.IsValid() {
		t.Fatal("MetadataResult is missing ReleaseDate")
	}
	return field
}

func setMetadataReleaseDate(t *testing.T, result *MetadataResult, value string) {
	t.Helper()

	field := metadataReleaseDateField(t, result)
	if !field.CanSet() {
		t.Fatal("MetadataResult.ReleaseDate is not settable")
	}
	field.SetString(value)
}

func getMetadataReleaseDate(t *testing.T, result *MetadataResult) string {
	t.Helper()

	return metadataReleaseDateField(t, result).String()
}

func TestMetadataResultToItem_CarriesReleaseDate(t *testing.T) {
	result := &MetadataResult{
		HasMetadata: true,
		Title:       "Movie",
	}
	setMetadataReleaseDate(t, result, "2024-01-02")

	item := metadataResultToItem(result, "movie")
	if item.ReleaseDate == nil {
		t.Fatal("expected item release_date to be set")
	}
	if got := *item.ReleaseDate; got != "2024-01-02" {
		t.Fatalf("expected item release_date 2024-01-02, got %q", got)
	}
}

func TestItemToMetadataResult_CarriesReleaseDate(t *testing.T) {
	releaseDate := "2024-01-02"
	result := itemToMetadataResult(&models.MediaItem{
		ContentID:   "movie-1",
		Type:        "movie",
		Title:       "Movie",
		ReleaseDate: &releaseDate,
	})

	if got := getMetadataReleaseDate(t, result); got != releaseDate {
		t.Fatalf("expected metadata release_date %q, got %q", releaseDate, got)
	}
}

func TestMergeMetadata_CarriesReleaseDate(t *testing.T) {
	source := &MetadataResult{}
	target := &MetadataResult{}
	setMetadataReleaseDate(t, source, "2024-01-02")

	MergeMetadata(source, target, nil, MergeFillEmpty)

	if got := getMetadataReleaseDate(t, target); got != "2024-01-02" {
		t.Fatalf("expected merged release_date 2024-01-02, got %q", got)
	}
}
