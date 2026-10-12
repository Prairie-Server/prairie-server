package apiv2

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	opReportClientDiagnosticEvents = "reportClientDiagnosticEvents"
	clientEventsMaxBodyBytes       = 8 << 10
	// ponytail: one in-process fixed window per account; a shared limiter only
	// if the server ever runs several API processes behind one account.
	clientEventsPerMinute = 30
)

// ClientDiagnosticEvent is one breadcrumb a client wants in the server log,
// such as a player state transition on a TV that has no other log channel.
type ClientDiagnosticEvent struct {
	Event             string   `json:"event" minLength:"1" maxLength:"64" pattern:"^[A-Za-z0-9_.:-]+$" doc:"Short event name; letters, digits and _ . : - only" example:"player_buffering_start"`
	OccurredAt        *Instant `json:"occurred_at,omitempty" nullable:"false" doc:"When it happened on the client; absent means when it was received"`
	PlaybackSessionID string   `json:"playback_session_id,omitempty" maxLength:"128" pattern:"^[A-Za-z0-9_.:-]+$" doc:"Playback or live session the event belongs to, for correlating with server playback logs"`
}

type ClientDiagnosticEventsBody struct {
	Events []ClientDiagnosticEvent `json:"events" minItems:"1" maxItems:"20"`
}

type ClientDiagnosticEventsInput struct {
	PlaybackRequestHeaders
	DeviceID string `header:"X-Prairie-Device-Id" maxLength:"128" doc:"The client's stable device identifier, logged with each event" example:"shield-living-room"`
	Body     ClientDiagnosticEventsBody
}

// ClientDiagnosticEventsReceipt acknowledges that the batch was logged. It is
// not stored anywhere else; a lost reply is not retried.
type ClientDiagnosticEventsReceipt struct {
	Accepted int    `json:"accepted" doc:"Events logged from this batch"`
	Outcome  string `json:"outcome" enum:"accepted"`
}
type ClientDiagnosticEventsOutput struct {
	Status int
	Body   ClientDiagnosticEventsReceipt
}

// clientEventLimiter is a per-account fixed one-minute window.
type clientEventLimiter struct {
	mu        sync.Mutex
	windows   map[int]clientEventWindow
	lastSweep time.Time
}

type clientEventWindow struct {
	start time.Time
	count int
}

func (l *clientEventLimiter) allow(userID int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[int]clientEventWindow{}
	}
	w := l.windows[userID]
	if now.Sub(w.start) >= time.Minute {
		w = clientEventWindow{start: now}
	}
	if w.count >= clientEventsPerMinute {
		return false
	}
	w.count++
	l.windows[userID] = w
	if len(l.windows) > 10_000 && now.Sub(l.lastSweep) >= time.Minute {
		l.lastSweep = now
		for id, candidate := range l.windows {
			if now.Sub(candidate.start) >= time.Minute {
				delete(l.windows, id)
			}
		}
	}
	return true
}

// registerClientDiagnosticEvents gives clients a sanctioned way to land short
// breadcrumbs in the server log (component client_diagnostics). It replaces
// the v1-era beacon that smuggled the event name into a settings query string.
// Nothing is persisted: this is not the consent-gated report upload.
func registerClientDiagnosticEvents(reg *Registry) {
	limiter := &clientEventLimiter{}
	op := Operation{Operation: humaOp(http.MethodPost, Prefix+"/diagnostics/client-events", opReportClientDiagnosticEvents, "diagnostics", "Log a short batch of client diagnostic breadcrumbs for this profile and device. Never retried automatically; a 429 means drop the batch."), Class: ClassProfileScoped, RetrySafety: RetrySafetyNonRetryable, MaxBodyBytes: clientEventsMaxBodyBytes}
	op.DefaultStatus = http.StatusAccepted
	op.Errors = []int{http.StatusUnprocessableEntity, http.StatusTooManyRequests}
	Register(reg, op, func(ctx context.Context, in *ClientDiagnosticEventsInput) (*ClientDiagnosticEventsOutput, error) {
		userID, profileID, p := viewerIdentity(ctx)
		if p != nil {
			return nil, p
		}
		if !limiter.allow(userID, time.Now()) {
			return nil, NewProblem(TypeRateLimited, fmt.Sprintf("At most %d client event batches a minute; drop this batch.", clientEventsPerMinute)).WithHeader("Retry-After", "60")
		}
		client := in.clientInfo()
		deviceID := strings.TrimSpace(in.DeviceID)
		received := time.Now().UTC()
		for _, event := range in.Body.Events {
			occurred := received
			if event.OccurredAt != nil {
				occurred = event.OccurredAt.Time
			}
			slog.InfoContext(ctx, "client diagnostic event",
				"component", "client_diagnostics",
				"event", event.Event,
				"occurred_at", occurred.Format(time.RFC3339Nano),
				"playback_session_id", event.PlaybackSessionID,
				"user_id", userID,
				"profile_id", profileID,
				"device_id", deviceID,
				"client", client.Name,
				"client_version", client.Version,
				"client_build", client.Build,
			)
		}
		return &ClientDiagnosticEventsOutput{Status: http.StatusAccepted, Body: ClientDiagnosticEventsReceipt{Accepted: len(in.Body.Events), Outcome: adminHistoryAccepted}}, nil
	})
}
