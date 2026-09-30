package playback

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestApplyCopyVideoMPEGTSV3(t *testing.T) {
	claimed := StartRequestV3{ClientPlaybackContext: ClientPlaybackContextV3{Deliveries: map[string]DeliveryCapabilityV3{
		DeliveryClassHLSV3: {Enabled: true, ValidatedClaims: []string{ClaimCopyVideoMPEGTSV3}},
	}}}
	unclaimed := StartRequestV3{ClientPlaybackContext: ClientPlaybackContextV3{Deliveries: map[string]DeliveryCapabilityV3{
		DeliveryClassHLSV3: {Enabled: true},
	}}}
	cases := []struct {
		name     string
		delivery DeliveryV3
		codec    string
		request  StartRequestV3
		want     bool
	}{
		{"hevc remux claimed", DeliveryRemuxHLSV3, "hevc", claimed, true},
		{"h264 remux claimed", DeliveryRemuxHLSV3, "h264", claimed, true},
		{"av1 has no TS mapping", DeliveryRemuxHLSV3, "av1", claimed, false},
		{"not claimed", DeliveryRemuxHLSV3, "hevc", unclaimed, false},
		{"transcode is already TS", DeliveryTranscodeHLSV3, "hevc", claimed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := PlannerResultV3{Plan: &PlanV3{Delivery: tc.delivery}}
			applyCopyVideoMPEGTSV3(&result, tc.request, &models.MediaFile{CodecVideo: tc.codec})
			if result.CopyVideoMPEGTS != tc.want {
				t.Fatalf("CopyVideoMPEGTS = %v, want %v", result.CopyVideoMPEGTS, tc.want)
			}
		})
	}
}

func TestExecutableRecipeV3KeepsCopyVideoMPEGTS(t *testing.T) {
	result := PlannerResultV3{Plan: &PlanV3{PlanID: "plan-00000001"}, PlayMethod: PlayRemux, CopyVideoMPEGTS: true}
	recipe := FreezeExecutableRecipeV3(result)
	if !recipe.Valid() {
		t.Fatalf("recipe invalid: %+v", recipe)
	}
	if !recipe.PlannerResult(result.Plan).CopyVideoMPEGTS {
		t.Fatal("thawed result lost CopyVideoMPEGTS")
	}
}
