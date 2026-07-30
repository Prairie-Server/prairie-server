package livetv

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// hdhomerunErrorHeader carries the tuner's own reason for refusing a stream.
//
// This is the only place the reason exists. FFmpeg reports the refusal as
// "Server returned 5XX Server Error reply" and discards the header, so a failed
// live session can only be explained by asking the tuner directly.
const hdhomerunErrorHeader = "X-HDHomeRun-Error"

// tunerDiagnoseTimeout bounds the explain-a-failure probe. It runs only after a
// session has already failed, so it must not add a noticeable delay to the error
// the user is waiting on.
const tunerDiagnoseTimeout = 5 * time.Second

// ErrNoSignal means the tuner locked the channel but got no usable video from
// the air. It is a reception problem -- aiming, cabling, or a marginal
// transmitter -- and no amount of retrying on our side changes it.
var ErrNoSignal = fmt.Errorf("%w: no signal on this channel", ErrNotFound)

// DescribeTunerRefusal explains why a tuner refused to stream sourceURL, by
// asking it and reading the reason out of its response.
//
// Returns nil when the tuner is reachable and willing, which means the failure
// was ours rather than the tuner's and the caller should keep its original
// error. Never returns the raw FFmpeg text: the point is to replace
// "exit status 8" with something a viewer can act on.
//
// Deliberately a post-failure diagnosis rather than a pre-flight check. Opening
// the stream URL claims a tuner, so probing before every session would race the
// FFmpeg process that is about to claim it for real -- on a busy device the probe
// could take the last tuner and make the very failure it was trying to describe.
func DescribeTunerRefusal(ctx context.Context, client *http.Client, sourceURL string) error {
	if strings.TrimSpace(sourceURL) == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{}
	}
	ctx, cancel := context.WithTimeout(ctx, tunerDiagnoseTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		// A canceled or timed-out probe learned nothing about the tuner, so it
		// must not manufacture a verdict. Reporting "not reachable" here would
		// overwrite the real startup error with a guess, and a tuner that is
		// merely slow would be blamed for a failure that was not its own.
		if ctx.Err() != nil {
			return nil
		}
		// The tuner is unreachable, which is itself the explanation.
		return fmt.Errorf("%w: tuner is not reachable", ErrNotFound)
	}
	// Close immediately: a successful GET has claimed a tuner, and holding it
	// would deny the resource we are only asking about.
	_ = resp.Body.Close()

	if resp.StatusCode < 400 {
		return nil
	}
	return tunerRefusalError(resp.StatusCode, resp.Header.Get(hdhomerunErrorHeader))
}

// tunerRefusalError maps a tuner's status and error header onto a sentinel the
// API layer already knows how to answer with.
//
// The numeric codes are HDHomeRun's own. They are matched on the code rather
// than the message text so a firmware wording change does not silently demote a
// mapped error back to a generic 500.
func tunerRefusalError(statusCode int, errorHeader string) error {
	code, message := parseHDHomeRunError(errorHeader)
	switch code {
	case "807":
		// Tuned, but nothing decodable arrived.
		return ErrNoSignal
	case "802", "803":
		// Resource locked / in use by another client or recording.
		return fmt.Errorf("%w: all tuners are in use", ErrNoTuner)
	}
	if message != "" {
		return fmt.Errorf("%w: tuner refused the channel: %s", ErrNotFound, message)
	}
	if statusCode == http.StatusServiceUnavailable {
		// 503 with no reason header is how older firmware reports contention.
		return fmt.Errorf("%w: tuner is unavailable", ErrNoTuner)
	}
	return fmt.Errorf("%w: tuner refused the channel (HTTP %d)", ErrNotFound, statusCode)
}

// parseHDHomeRunError splits "807 No Video Data" into its code and message.
// Either half may be absent; a header with no leading numeric code is treated as
// all message, so the text still reaches the user.
func parseHDHomeRunError(header string) (code, message string) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ""
	}
	// Split on whitespace rather than a literal space: HTTP allows a tab to
	// separate header tokens, and matching only " " would leave "807\tNo Video
	// Data" looking like an unmapped message and quietly lose its ErrNoSignal
	// mapping.
	fields := strings.Fields(header)
	code = fields[0]
	if !isAllDigits(code) {
		return "", header
	}
	return code, strings.Join(fields[1:], " ")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
