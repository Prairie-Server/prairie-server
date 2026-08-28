package artworkkey

import (
	"reflect"
	"slices"
	"testing"
)

// Stills lost their w200 rung because nothing ever asked for one: the server
// keeps stills at w500 for televisions and no caller passes size="small" for a
// still. Posters and profiles keep it — that is the TV rung.
func TestVariantWidthsStillHasNoTVRung(t *testing.T) {
	if got := VariantWidths("still"); !reflect.DeepEqual(got, []int{500, 300}) {
		t.Errorf("VariantWidths(still) = %v, want [500 300]", got)
	}
	for _, imageType := range []string{"poster", "profile"} {
		if got := VariantWidths(imageType); !slices.Contains(got, 200) {
			t.Errorf("VariantWidths(%s) = %v, want the w200 TV rung retained", imageType, got)
		}
	}
	if got := VariantWidths("backdrop"); !reflect.DeepEqual(got, []int{1920, 1280, 300}) {
		t.Errorf("VariantWidths(backdrop) = %v, want [1920 1280 300]", got)
	}
	if got := VariantWidths("logo"); !reflect.DeepEqual(got, []int{500}) {
		t.Errorf("VariantWidths(logo) = %v, want [500]", got)
	}
}

// A retired width must never also be a live one, or the sweeper would delete
// objects the generator is still producing.
func TestRetiredVariantWidthsNeverOverlapLiveLadder(t *testing.T) {
	for _, imageType := range []string{"poster", "still", "profile", "backdrop", "logo", "", "unknown"} {
		live := VariantWidths(imageType)
		for _, retired := range RetiredVariantWidths(imageType) {
			if slices.Contains(live, retired) {
				t.Errorf("%s: w%d is both live and retired", imageType, retired)
			}
		}
	}
}

func TestRetiredVariantWidthsRecordsTheStillRung(t *testing.T) {
	if got := RetiredVariantWidths("still"); !reflect.DeepEqual(got, []int{200}) {
		t.Errorf("RetiredVariantWidths(still) = %v, want [200]", got)
	}
	for _, imageType := range []string{"poster", "profile", "backdrop", "logo"} {
		if got := RetiredVariantWidths(imageType); len(got) != 0 {
			t.Errorf("RetiredVariantWidths(%s) = %v, want none", imageType, got)
		}
	}
}

func TestRetiredVariantKeysCoversEverySiblingFormat(t *testing.T) {
	keys := RetiredVariantKeys("artwork/tv/ep1/still/original.7.webp", "still")
	want := []string{
		"artwork/tv/ep1/still/w200.7.webp",
		"artwork/tv/ep1/still/w200.7.avif",
		"artwork/tv/ep1/still/w200.7.png",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("RetiredVariantKeys = %v, want %v", keys, want)
	}
}

// Types that never retired a rung must produce no keys at all, so the sweeper
// spends no HEAD requests on them.
func TestRetiredVariantKeysEmptyForTypesWithNoHistory(t *testing.T) {
	for _, imageType := range []string{"poster", "profile", "backdrop", "logo"} {
		if got := RetiredVariantKeys("artwork/x/"+imageType+"/original.1.webp", imageType); len(got) != 0 {
			t.Errorf("RetiredVariantKeys(%s) = %v, want none", imageType, got)
		}
	}
}

func TestRetiredVariantKeysIgnoresEmptyPath(t *testing.T) {
	if got := RetiredVariantKeys("   ", "still"); got != nil {
		t.Errorf("RetiredVariantKeys with a blank path = %v, want nil", got)
	}
}

// Dropping w200 from the ladder must not strand a client that still asks for
// one: the fallback offers the wider rungs that do exist.
func TestWiderVariantKeysStillCoversARetiredStillRequest(t *testing.T) {
	keys := WiderVariantKeys("artwork/tv/ep1/still/w200.7.webp")
	if len(keys) == 0 {
		t.Fatal("a w200 still request has no wider rung to fall back to")
	}
	for _, key := range keys {
		if key == "artwork/tv/ep1/still/w200.7.webp" {
			t.Error("fallback offered the retired rung itself")
		}
	}
}

// VariantNames drives generation and existence checks; it must not name the
// retired rung.
func TestVariantNamesForStillOmitsRetiredRung(t *testing.T) {
	for _, name := range VariantNames("still") {
		if name == "w200" {
			t.Fatal("VariantNames(still) still names w200, so it would be regenerated")
		}
	}
	if got := VariantNames("still"); !reflect.DeepEqual(got, []string{OriginalVariant, "w500", "w300"}) {
		t.Errorf("VariantNames(still) = %v", got)
	}
}
