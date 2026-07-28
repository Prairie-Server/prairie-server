package livetv

import "testing"

// Sending ?transcode= to a tuner that does not advertise it is silently
// ignored, so the capability has to be explicit rather than assumed.
func TestTunerSupportsDeviceTranscode(t *testing.T) {
	if (Tuner{}).SupportsDeviceTranscode() {
		t.Fatal("a tuner reporting nothing must not claim device transcoding")
	}
	if !(Tuner{TranscodeCodecs: []string{"heavy"}}).SupportsDeviceTranscode() {
		t.Fatal("an advertised profile should count as supported")
	}
}

func TestCodecListRoundTrip(t *testing.T) {
	if got := joinCodecList([]string{"heavy", " mobile ", ""}); got != "heavy,mobile" {
		t.Fatalf("joinCodecList = %q", got)
	}
	if got := joinCodecList(nil); got != "" {
		t.Fatalf("joinCodecList(nil) = %q, want empty", got)
	}

	got := splitCodecList("heavy, mobile ,")
	if len(got) != 2 || got[0] != "heavy" || got[1] != "mobile" {
		t.Fatalf("splitCodecList = %v", got)
	}
	if splitCodecList("") != nil || splitCodecList("  ") != nil || splitCodecList(",") != nil {
		t.Fatal("blank capability lists should decode to nil")
	}
}
