package playback

import (
	"strings"
	"testing"
	"time"
)

func testEngine() (*AdviceEngine, *time.Time) {
	clock := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	e := NewAdviceEngine()
	e.now = func() time.Time { return clock }
	return e, &clock
}

// A rung whose resolution resolutionToScale does not understand silently
// produces no scale filter, so the "encode" runs at source resolution and the
// viewer gets none of the bandwidth relief they asked for. This is the real
// ladder invariant -- not NormalizeQualityV3 conformance, which governs
// quality_preference rather than target_resolution.
func TestQualityLadderResolutionsAreScalable(t *testing.T) {
	for _, rung := range QualityLadderFor(0) {
		if resolutionToScale(rung.Resolution) == "" {
			t.Errorf("rung %q has resolution %q, which resolutionToScale does not handle; an encode there would not downscale",
				rung.ID, rung.Resolution)
		}
		if got, ok := RungByID(rung.ID); !ok || got.ID != rung.ID {
			t.Errorf("RungByID(%q) = (%+v, %v)", rung.ID, got, ok)
		}
	}
}

// Rung ids must be unique, since clients key their selection on them. Compared
// case-insensitively because RungByID and the step helpers use strings.EqualFold:
// two rungs differing only in case would collide there while looking distinct in
// a case-sensitive check.
func TestQualityLadderIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, rung := range QualityLadderFor(0) {
		key := strings.ToLower(rung.ID)
		if seen[key] {
			t.Errorf("duplicate rung id %q (case-insensitively)", rung.ID)
		}
		seen[key] = true
	}
}

// Bitrate disambiguates rungs sharing a resolution, and a cap matching no rung
// exactly resolves to the nearest one so sessions started before a ladder change
// still get advice.
func TestRungForSessionDisambiguatesByBitrate(t *testing.T) {
	for _, tc := range []struct {
		resolution string
		bitrate    int
		wantID     string
	}{
		{resolution: resolution1080p, bitrate: 10000, wantID: "1080p-high"},
		{resolution: resolution1080p, bitrate: 6000, wantID: "1080p"},
		{resolution: resolution1080p, bitrate: 9000, wantID: "1080p-high"},
		{resolution: "720p", bitrate: 4000, wantID: "720p-high"},
		{resolution: "720p", bitrate: 2000, wantID: "720p"},
		{resolution: "720p", bitrate: 0, wantID: "720p"},
	} {
		got, ok := RungForSession(tc.resolution, tc.bitrate)
		if !ok || got.ID != tc.wantID {
			t.Errorf("RungForSession(%q, %d) = (%q, %v), want %q",
				tc.resolution, tc.bitrate, got.ID, ok, tc.wantID)
		}
	}
}

// Offering a rung above the source would spend encode time upscaling detail the
// file does not contain, and misrepresents what the viewer gets.
func TestQualityLadderForNeverOffersAboveSource(t *testing.T) {
	for _, tc := range []struct {
		sourceHeight int
		wantTop      string
		wantLen      int
	}{
		{sourceHeight: 2160, wantTop: "2160p", wantLen: 7},
		{sourceHeight: 1080, wantTop: "1080p-high", wantLen: 6},
		{sourceHeight: 720, wantTop: "720p-high", wantLen: 4},
		{sourceHeight: 480, wantTop: "480p", wantLen: 2},
		// Mastered slightly under a rung: the source keeps its own rung.
		{sourceHeight: 1072, wantTop: "1080p-high", wantLen: 6},
		// Below the floor still yields the floor, so Auto has somewhere to go.
		{sourceHeight: 240, wantTop: "420p", wantLen: 1},
		// Unknown dimensions: full ladder beats hiding the source's own rung.
		{sourceHeight: 0, wantTop: "2160p", wantLen: 7},
	} {
		ladder := QualityLadderFor(tc.sourceHeight)
		if len(ladder) != tc.wantLen || ladder[0].ID != tc.wantTop {
			t.Errorf("QualityLadderFor(%d) = %d rungs topped by %q, want %d topped by %q",
				tc.sourceHeight, len(ladder), ladder[0].ID, tc.wantLen, tc.wantTop)
		}
	}
}

// Modes are not rungs: none names a resolution a measurement could be compared
// against, so treating one as a rung would step from a fiction.
func TestRungForSessionRejectsModes(t *testing.T) {
	for _, mode := range []string{"", "   ", "auto", "original", "source", "max", "nonsense"} {
		if rung, ok := RungForSession(mode, 6000); ok {
			t.Errorf("RungForSession(%q) returned rung %+v, want no rung", mode, rung)
		}
	}
}

func TestAdviceEngineDownshiftsAfterSustainedRebuffering(t *testing.T) {
	e, _ := testEngine()
	report := DeliveryReport{Resolution: resolution2160p, BitrateKbps: 20000, SourceHeight: 2160, ThroughputKbps: 5000, Rebuffering: true}

	// One bad report is not enough: a single rebuffer happens on any link.
	if advice, ok := e.Observe("s1", report); ok {
		t.Fatalf("advised on first unhealthy report: %+v", advice)
	}
	advice, ok := e.Observe("s1", report)
	if !ok {
		t.Fatal("no advice after sustained rebuffering")
	}
	if advice.Direction != "down" || advice.Reason != "rebuffering" {
		t.Errorf("advice = %+v, want a downshift for rebuffering", advice)
	}
	// 5000kbps supports 1080p (6000) only by over-reach, so the fitted rung is
	// 720p -- reached in one move rather than one rebuffer per step.
	if advice.RungID != "720p-high" {
		t.Errorf("RungID = %q, want 720p-high fitted to observed throughput", advice.RungID)
	}
}

// The failure mode that would make this worse than nothing: advising a switch,
// then advising the reverse as soon as conditions momentarily look good.
func TestAdviceEngineDoesNotOscillate(t *testing.T) {
	e, clock := testEngine()
	bad := DeliveryReport{Resolution: resolution1080p, BitrateKbps: 6000, SourceHeight: 2160, ThroughputKbps: 1000, Rebuffering: true}

	e.Observe("s1", bad)
	first, ok := e.Observe("s1", bad)
	if !ok || first.Direction != "down" {
		t.Fatalf("expected a downshift, got (%+v, %v)", first, ok)
	}

	// Immediately healthy and plentiful, at the rung we were told to use.
	good := DeliveryReport{Resolution: first.Resolution, BitrateKbps: first.BitrateKbps, SourceHeight: 2160, ThroughputKbps: 100000}
	for i := 0; i < adviceUpshiftSamples+2; i++ {
		if advice, ok := e.Observe("s1", good); ok {
			t.Fatalf("advised again during cooldown (iteration %d): %+v", i, advice)
		}
	}

	// Past the cooldown, a genuinely fast link may climb -- that is not
	// oscillation, it is recovery, and it took the full healthy run to earn.
	*clock = clock.Add(adviceCooldown + time.Second)
	for i := 0; i < adviceUpshiftSamples; i++ {
		e.Observe("s1", good)
	}
	if advice, ok := e.Observe("s1", good); ok && advice.Direction != "up" {
		t.Errorf("post-cooldown advice = %+v, want up or none", advice)
	}
}

// Absence of rebuffering is not evidence a higher rung would survive.
func TestAdviceEngineNeverClimbsWithoutHeadroom(t *testing.T) {
	e, _ := testEngine()
	// Enough to sustain 720p, nowhere near 1.4x of 1080p's 6000.
	marginal := DeliveryReport{Resolution: "720p", BitrateKbps: 2000, SourceHeight: 2160, ThroughputKbps: 3200}
	for i := 0; i < adviceUpshiftSamples*3; i++ {
		if advice, ok := e.Observe("s1", marginal); ok {
			t.Fatalf("climbed without headroom on iteration %d: %+v", i, advice)
		}
	}

	// Nor on no measurement at all.
	e2, _ := testEngine()
	unmeasured := DeliveryReport{Resolution: "720p", BitrateKbps: 2000, SourceHeight: 2160}
	for i := 0; i < adviceUpshiftSamples*3; i++ {
		if advice, ok := e2.Observe("s2", unmeasured); ok {
			t.Fatalf("climbed with no throughput measurement: %+v", advice)
		}
	}
}

// At the floor there is nothing to give, and repeating that costs a replan slot
// and a rebuffer to arrive where playback already is.
func TestAdviceEngineSilentAtLadderFloor(t *testing.T) {
	e, _ := testEngine()
	report := DeliveryReport{Resolution: "420p", BitrateKbps: 720, SourceHeight: 2160, ThroughputKbps: 100, Rebuffering: true}
	for i := 0; i < 10; i++ {
		if advice, ok := e.Observe("s1", report); ok {
			t.Fatalf("advised at ladder floor: %+v", advice)
		}
	}
}

// A paused player is stopped, not starved; its throughput says nothing.
func TestAdviceEngineIgnoresPausedAndUnknownRungs(t *testing.T) {
	e, _ := testEngine()
	paused := DeliveryReport{Resolution: resolution2160p, BitrateKbps: 20000, SourceHeight: 2160, Rebuffering: true, Paused: true}
	for i := 0; i < 5; i++ {
		if advice, ok := e.Observe("s1", paused); ok {
			t.Fatalf("advised while paused: %+v", advice)
		}
	}

	// Direct play / auto sessions have no rung to step from.
	for _, res := range []string{"", "auto", "original"} {
		report := DeliveryReport{Resolution: res, SourceHeight: 2160, Rebuffering: true}
		for i := 0; i < 5; i++ {
			if advice, ok := e.Observe("s2", report); ok {
				t.Fatalf("advised for non-ladder resolution %q: %+v", res, advice)
			}
		}
	}
}

// Stale history must not be applied to a resumed session: a viewer who paused
// for ten minutes should not be judged on pre-pause throughput.
func TestAdviceEngineDiscardsStaleHistory(t *testing.T) {
	e, clock := testEngine()
	bad := DeliveryReport{Resolution: resolution1080p, BitrateKbps: 6000, SourceHeight: 2160, ThroughputKbps: 500, Rebuffering: true}

	e.Observe("s1", bad)
	*clock = clock.Add(adviceSampleTTL + time.Minute)

	// The stale unhealthy sample is discarded, so this counts as the first one
	// again and cannot on its own trigger advice.
	if advice, ok := e.Observe("s1", bad); ok {
		t.Fatalf("stale history carried into a resumed session: %+v", advice)
	}
}

func TestAdviceEngineForgetClearsSession(t *testing.T) {
	e, _ := testEngine()
	bad := DeliveryReport{Resolution: resolution1080p, BitrateKbps: 6000, SourceHeight: 2160, ThroughputKbps: 500, Rebuffering: true}

	e.Observe("s1", bad)
	e.Forget("s1")
	if advice, ok := e.Observe("s1", bad); ok {
		t.Fatalf("advice survived Forget: %+v", advice)
	}
}

// A nil engine must be usable, so wiring can land before construction does.
func TestAdviceEngineNilSafe(t *testing.T) {
	var e *AdviceEngine
	if _, ok := e.Observe("s1", DeliveryReport{Resolution: resolution1080p, BitrateKbps: 6000, Rebuffering: true}); ok {
		t.Error("nil engine returned advice")
	}
	e.Forget("s1")
}

func TestQualityOptionsForLeadsWithModes(t *testing.T) {
	opts := QualityOptionsFor(1080)
	if len(opts.Modes) == 0 || opts.Modes[0] != "auto" {
		t.Errorf("Modes = %v, want auto first", opts.Modes)
	}
	if len(opts.Rungs) != 6 || opts.Rungs[0].ID != "1080p-high" {
		t.Errorf("Rungs = %+v, want 1080p-capped ladder", opts.Rungs)
	}
	if opts.SourceHeight != 1080 {
		t.Errorf("SourceHeight = %d, want 1080", opts.SourceHeight)
	}
}

func TestIsLadderMode(t *testing.T) {
	for _, mode := range []string{"", "auto", "AUTO", " original ", "source", "max"} {
		if !IsLadderMode(mode) {
			t.Errorf("IsLadderMode(%q) = false, want true", mode)
		}
	}
	for _, rung := range []string{"1080p", "720p", "480p", "420p", resolution2160p} {
		if IsLadderMode(rung) {
			t.Errorf("IsLadderMode(%q) = true, want false", rung)
		}
	}
}
