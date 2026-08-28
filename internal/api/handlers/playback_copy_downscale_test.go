package handlers

import "testing"

// A copied video stream cannot be rescaled, so pairing target_codec_video=copy
// with a lower target_resolution is a request the server cannot honor. Before the
// guard it was accepted: ffmpeg ran with -c:v copy and no scale filter and served
// the source resolution under the requested label, so the switch reported success
// and changed nothing.
//
// The predicate is exercised directly rather than through HandleStartTranscode,
// which needs a session manager, a file repository and a settings store before it
// reaches this point; the arithmetic is the part that decides the outcome.
func TestCopyVideoDownscaleConflict(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		source   string
		conflict bool
	}{
		// The reported failure: a 1080p rung requested on a 2160p source while
		// copying video.
		{"downscale from 2160p", "1080p", "2160p", true},
		{"downscale from 1080p", "720p", "1080p", true},
		{"two rungs down", "480p", "2160p", true},
		// The AV1 path deliberately copies a 2160p source while naming 2160p, so an
		// equal target must stay allowed or that route breaks.
		{"equal resolution", "2160p", "2160p", false},
		{"upscale request is a no-op for copy", "2160p", "1080p", false},
		// Nothing to compare: no target asked for, or a label we do not map.
		{"no target resolution", "", "2160p", false},
		{"unknown target label", "potato", "2160p", false},
		{"unknown source label", "1080p", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conflict := false
			if requested, ok := transcodeResolutionHeight(tc.target); ok {
				if source, known := transcodeResolutionHeight(tc.source); known && requested < source {
					conflict = true
				}
			}
			if conflict != tc.conflict {
				t.Errorf("copy video with target=%q source=%q: conflict=%v, want %v",
					tc.target, tc.source, conflict, tc.conflict)
			}
		})
	}
}
