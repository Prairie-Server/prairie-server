package playback

import (
	"strings"
	"sync"
	"time"
)

// Advice is a recommendation for what quality a session should switch to.
//
// Advisory only. The server cannot change a stream a client is already reading
// -- a progressive body cannot be swapped mid-flight and a media playlist is
// pinned to its session -- so acting on this means the client calls
// /playback/{id}/replan. A client that ignores advice keeps playing exactly as
// before, which is why this can ship before any client honors it.
type Advice struct {
	// Resolution is the ladder token to replan to, e.g. "720p".
	Resolution string `json:"resolution"`
	// BitrateKbps is that rung's cap, so a client can pass it as
	// bandwidth_cap_kbps without knowing the ladder itself.
	BitrateKbps int `json:"bitrate_kbps"`
	// Direction is "down" or "up", for logging and for clients that only want
	// to honor downshifts (the safe half: a downshift fixes a stall, an upshift
	// only risks creating one).
	Direction string `json:"direction"`
	// Reason is a stable machine-readable cause, e.g. "rebuffering".
	Reason string `json:"reason"`
	// ObservedKbps is the throughput the decision was made on, so a client can
	// show the viewer why quality changed instead of it looking arbitrary.
	ObservedKbps int `json:"observed_kbps"`
}

// Delivery health thresholds.
//
// These are intentionally asymmetric. Downshifting is cheap and fixes a viewer
// who is currently stalled, so it happens after a short confirmation. Upshifting
// only risks re-creating the stall we just escaped, so it demands a longer clean
// run and more headroom. Symmetric thresholds oscillate: a link that just barely
// sustained a rung is exactly the link that stalls again once we return to it.
const (
	// adviceDownshiftSamples is how many consecutive unhealthy reports are
	// needed before recommending a downshift. One is too eager -- a single
	// rebuffer happens on any link when a keyframe lands badly.
	adviceDownshiftSamples = 2
	// adviceUpshiftSamples is the consecutive healthy reports needed to climb.
	adviceUpshiftSamples = 6
	// adviceUpshiftHeadroom is the multiple of the target rung's bitrate that
	// measured throughput must clear before climbing to it. 1.0 would mean
	// recommending a rung the link can only just carry.
	adviceUpshiftHeadroom = 1.4
	// adviceCooldown is the quiet period after any recommendation. Replan has a
	// concurrency semaphore and each switch costs the viewer a rebuffer, so
	// advice must not be issued faster than a client could act on it.
	adviceCooldown = 30 * time.Second
	// adviceSampleTTL is how long a session's history stays relevant. A viewer
	// who paused for ten minutes should not be judged on pre-pause throughput.
	adviceSampleTTL = 2 * time.Minute
	// adviceThroughputAlpha weights the newest sample in the throughput EWMA.
	// Low enough to ignore a single slow segment, high enough to react within a
	// few reports.
	adviceThroughputAlpha = 0.4
)

// DeliveryReport is one observation of how a session is being delivered.
//
// Reported by the client because that is the only place the truth lives: the
// server knows how fast it wrote bytes, but not whether the player's buffer ran
// dry, and a client can be stalled on its own decode while the socket looks
// perfectly healthy.
type DeliveryReport struct {
	// Resolution is the rung currently playing, needed to know which way to
	// step. Empty means unknown and no advice is possible.
	Resolution string
	// SourceHeight bounds upshifts to what the file actually contains.
	SourceHeight int
	// ThroughputKbps is the client's measured download rate. Zero means the
	// client did not measure, and only buffering state is considered.
	ThroughputKbps int
	// Rebuffering reports that playback is currently starved.
	Rebuffering bool
	// Paused suppresses advice entirely: a paused player is not starved, it is
	// stopped, and its throughput says nothing about the link.
	Paused bool
}

type adviceState struct {
	throughputKbps float64
	unhealthy      int
	healthy        int
	lastAdviceAt   time.Time
	lastReportAt   time.Time
}

// AdviceEngine tracks per-session delivery health and recommends ladder moves.
//
// Lives server-side so every client benefits from one implementation: the
// alternative is per-platform ABR in web, Tizen, webOS, Android and Apple, five
// times the logic and five times the divergence. It also lets the decision use
// what only the server knows -- the real ladder, the encoder's capabilities and
// current transcode load -- rather than what a player can infer.
type AdviceEngine struct {
	mu       sync.Mutex
	sessions map[string]*adviceState
	// now is injectable so tests can drive cooldowns and TTLs without sleeping.
	now func() time.Time
}

func NewAdviceEngine() *AdviceEngine {
	return &AdviceEngine{sessions: make(map[string]*adviceState), now: time.Now}
}

// Forget drops a session's history. Called when playback stops so a finished
// session cannot advise a later one that reuses the id.
func (e *AdviceEngine) Forget(sessionID string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	delete(e.sessions, sessionID)
	e.mu.Unlock()
}

// Observe records a delivery report and returns advice when one is warranted.
//
// Reports false in the common case. Advice is the exception -- steady playback
// on a sufficient link should never produce one, because every recommendation a
// client acts on costs the viewer a rebuffer.
func (e *AdviceEngine) Observe(sessionID string, report DeliveryReport) (Advice, bool) {
	if e == nil || sessionID == "" || report.Paused {
		return Advice{}, false
	}
	current, ok := RungFor(report.Resolution)
	if !ok {
		// "auto"/"original"/direct-play sessions have no rung to step from. A
		// direct play is the source file itself; degrading it is a plan-level
		// decision, not a ladder move.
		return Advice{}, false
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()
	state := e.sessions[sessionID]
	if state == nil || now.Sub(state.lastReportAt) > adviceSampleTTL {
		// First report, or history too stale to trust. Seed the EWMA with the
		// observation rather than 0, so one sample does not read as a stall.
		state = &adviceState{throughputKbps: float64(report.ThroughputKbps)}
		e.sessions[sessionID] = state
	}
	state.lastReportAt = now

	if report.ThroughputKbps > 0 {
		state.throughputKbps = adviceThroughputAlpha*float64(report.ThroughputKbps) +
			(1-adviceThroughputAlpha)*state.throughputKbps
	}
	observed := int(state.throughputKbps)

	// Unhealthy means starved now, or measurably unable to sustain the rung.
	// Rebuffering counts on its own: a client can stall for reasons throughput
	// never reveals, and it is the symptom the viewer actually experiences.
	unhealthy := report.Rebuffering ||
		(report.ThroughputKbps > 0 && observed < current.BitrateKbps)
	if unhealthy {
		state.unhealthy++
		state.healthy = 0
	} else {
		state.healthy++
		state.unhealthy = 0
	}

	if !state.lastAdviceAt.IsZero() && now.Sub(state.lastAdviceAt) < adviceCooldown {
		return Advice{}, false
	}

	if state.unhealthy >= adviceDownshiftSamples {
		return e.adviseDown(state, now, current, report, observed)
	}
	if state.healthy >= adviceUpshiftSamples {
		return e.adviseUp(state, now, current, report, observed)
	}
	return Advice{}, false
}

func (e *AdviceEngine) adviseDown(
	state *adviceState, now time.Time, current QualityRung, report DeliveryReport, observed int,
) (Advice, bool) {
	// Jump straight to a rung the measurement supports rather than stepping
	// once per rebuffer. A client on a link carrying 2 Mbps should not endure
	// 4K -> 1080p -> 720p, three switches and three rebuffers, to get there.
	target, ok := StepDownFrom(current.Resolution, report.SourceHeight)
	if !ok {
		// Already at the floor: nothing to recommend, and saying so repeatedly
		// would spend replan slots to stand still. Reset so the next genuine
		// change of conditions is evaluated fresh.
		state.unhealthy = 0
		return Advice{}, false
	}
	if observed > 0 {
		if fitted, fitOK := HighestRungWithin(observed, report.SourceHeight); fitOK && fitted.Height < target.Height {
			target = fitted
		}
	}

	reason := "insufficient_throughput"
	if report.Rebuffering {
		reason = "rebuffering"
	}
	state.lastAdviceAt = now
	state.unhealthy = 0
	return Advice{
		Resolution:   target.Resolution,
		BitrateKbps:  target.BitrateKbps,
		Direction:    "down",
		Reason:       reason,
		ObservedKbps: observed,
	}, true
}

func (e *AdviceEngine) adviseUp(
	state *adviceState, now time.Time, current QualityRung, report DeliveryReport, observed int,
) (Advice, bool) {
	target, ok := StepUpFrom(current.Resolution, report.SourceHeight)
	if !ok {
		// Already at the source's best rung. Reset so the counter does not sit
		// pinned at the threshold, re-firing the moment a cooldown lapses.
		state.healthy = 0
		return Advice{}, false
	}
	// Never climb on no measurement: absence of rebuffering is not evidence a
	// higher rung would survive, and a wrong upshift manufactures the stall we
	// are trying to avoid.
	if observed <= 0 || float64(observed) < adviceUpshiftHeadroom*float64(target.BitrateKbps) {
		return Advice{}, false
	}

	state.lastAdviceAt = now
	state.healthy = 0
	return Advice{
		Resolution:   target.Resolution,
		BitrateKbps:  target.BitrateKbps,
		Direction:    "up",
		Reason:       "headroom_available",
		ObservedKbps: observed,
	}, true
}

// QualityOptions is the picker payload: the rungs a session can offer plus the
// modes that are not rungs.
type QualityOptions struct {
	Rungs        []QualityRung `json:"rungs"`
	Modes        []string      `json:"modes"`
	SourceHeight int           `json:"source_height,omitempty"`
}

// QualityOptionsFor builds the picker payload for a source height.
//
// "auto" and "original" lead because they are the two answers most viewers want:
// let the server decide, or do not touch my file.
func QualityOptionsFor(sourceHeight int) QualityOptions {
	return QualityOptions{
		Rungs:        QualityLadderFor(sourceHeight),
		Modes:        []string{"auto", "original"},
		SourceHeight: sourceHeight,
	}
}

// IsLadderMode reports whether a preference is a mode rather than a rung.
func IsLadderMode(preference string) bool {
	switch strings.ToLower(strings.TrimSpace(preference)) {
	case "", "auto", "original", "source", "max":
		return true
	default:
		return false
	}
}
