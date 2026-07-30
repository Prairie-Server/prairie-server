package livetv

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prairie-server/prairie-server/internal/idgen"
	"github.com/prairie-server/prairie-server/internal/playback"
)

// liveHLSPathPrefix is the route the live HLS bridge is served from. Shared by
// the URL builder and the delivery-ID parser so the two cannot drift.
const liveHLSPathPrefix = "/api/v1/livetv/live-hls/"

// PublicLiveHLSPath is the authenticated relative URL for a live HLS remux.
func PublicLiveHLSPath(playbackSessionID string) string {
	return liveHLSPathPrefix + playbackSessionID + "/index.m3u8"
}

// LiveHLSDeliveryID reports the playback session ID a live HLS URL delivers, and
// whether the URL is a live HLS path at all.
//
// Callers use it to decide whether a URL can carry a stream token and what that
// token must be bound to. The other client-safe play URL -- the MPEG-TS session
// proxy -- is keyed on a different ID, so returning false here keeps a token from
// being minted against the wrong one.
func LiveHLSDeliveryID(rawURL string) (string, bool) {
	idx := strings.Index(rawURL, liveHLSPathPrefix)
	if idx < 0 {
		return "", false
	}
	rest := rawURL[idx+len(liveHLSPathPrefix):]
	// Stop at a query string so an already-parameterized URL still resolves.
	if q := strings.IndexAny(rest, "?#"); q >= 0 {
		rest = rest[:q]
	}
	id, _, found := strings.Cut(rest, "/")
	if !found || id == "" {
		return "", false
	}
	return id, true
}

// DefaultMaxLiveTranscodes bounds concurrent re-encoding live sessions when the
// server does not configure a limit. Copy sessions are nearly free and stay
// ungated; an encode costs a CPU core (or an NVENC session) apiece.
const DefaultMaxLiveTranscodes = 3

// HLSBridge packages a tuner's MPEG-TS into sliding-window HLS for Prairie
// clients, copying the broadcast streams when the client can decode them and
// re-encoding the ones it cannot.
type HLSBridge struct {
	root       string
	ffmpegPath string
	// settings is read per tune so admin changes apply to the next channel
	// start rather than waiting for a restart.
	settings SettingsProvider
	// httpClient asks the tuner why it refused a channel. Nil is fine --
	// DescribeTunerRefusal falls back to a default client.
	httpClient *http.Client

	mu sync.Mutex
	// activeTranscodes counts encoding sessions against the configured cap.
	activeTranscodes int
	sessions         map[string]*bridgeSession
}

type bridgeSession struct {
	live      *playback.LiveHLSSession
	dir       string
	userID    int
	profileID string
	// holdsTranscodeSlot marks sessions that must return a slot when they stop.
	holdsTranscodeSlot bool
}

// HLSBridgeOptions configures a live HLS bridge.
type HLSBridgeOptions struct {
	// Root is the transcode working directory; sessions live under root/livetv-hls.
	Root string
	// FFmpegPath optionally overrides the ffmpeg binary.
	FFmpegPath string
	// HWAccel mirrors playback.hw_accel so live encodes use the same GPU path
	// as movie transcodes ("auto" probes the host). Ignored when Settings is
	// set, which carries the Live TV specific policy.
	HWAccel string
	// MaxTranscodes bounds concurrent encoding sessions; 0 uses
	// DefaultMaxLiveTranscodes and a negative value disables the limit.
	// Ignored when Settings is set.
	MaxTranscodes int
	// Settings supplies operator policy per tune. Optional.
	Settings SettingsProvider
}

// NewHLSBridge creates a live HLS playback bridge under Root/livetv-hls.
func NewHLSBridge(opts HLSBridgeOptions) *HLSBridge {
	root := opts.Root
	if root == "" {
		root = filepath.Join(os.TempDir(), "prairie-transcode")
	}
	settings := opts.Settings
	if settings == nil {
		static := TranscodeSettings{
			HWAccel:       opts.HWAccel,
			MaxTranscodes: opts.MaxTranscodes,
		}
		settings = func(context.Context) TranscodeSettings { return static }
	}
	return &HLSBridge{
		root:       filepath.Join(root, "livetv-hls"),
		ffmpegPath: opts.FFmpegPath,
		settings:   settings,
		sessions:   map[string]*bridgeSession{},
	}
}

func (b *HLSBridge) currentSettings(ctx context.Context) TranscodeSettings {
	if b == nil || b.settings == nil {
		return TranscodeSettings{}
	}
	return b.settings(ctx)
}

// LiveStreamRequest describes one client's tune.
type LiveStreamRequest struct {
	ChannelID string
	SourceURL string
	UserID    int
	ProfileID string
	// Plan is the per-stream copy-or-encode decision for this client.
	Plan StreamPlan
}

func (b *HLSBridge) StartLiveStream(
	ctx context.Context,
	req LiveStreamRequest,
) (playbackSessionID, playbackURL string, err error) {
	if b == nil {
		return "", "", fmt.Errorf("hls bridge not configured")
	}
	if err := ValidateMediaFetchURL(req.SourceURL); err != nil {
		return "", "", err
	}
	settings := b.currentSettings(ctx)
	plan := settings.applyTo(req.Plan)
	if plan.VideoCodec == "" {
		plan.VideoCodec = "copy"
	}
	if plan.AudioCodec == "" {
		plan.AudioCodec = "copy"
	}
	transcoding := plan.Transcodes()
	if transcoding && !b.acquireTranscodeSlot(settings.MaxTranscodes) {
		return "", "", fmt.Errorf("%w: live transcode capacity reached", ErrLimitExceeded)
	}
	id, err := idgen.NextID()
	if err != nil {
		b.releaseTranscodeSlot(transcoding)
		return "", "", err
	}
	// Both routes build a lead the player can spend before it needs the next
	// segment. A copy is realtime with respect to the source, which says nothing
	// about what the client has buffered, so it gets a lead too.
	lead := DefaultLiveCopyLeadSegments
	if transcoding {
		lead = DefaultLiveTranscodeLeadSegments
	}
	dir := filepath.Join(b.root, id)
	live, err := playback.StartLiveHLS(ctx, playback.LiveHLSOpts{
		ID:               id,
		InputURL:         req.SourceURL,
		OutputDir:        dir,
		FFmpegPath:       b.ffmpegPath,
		VideoCodec:       plan.VideoCodec,
		AudioCodec:       plan.AudioCodec,
		AudioChannels:    plan.AudioChannels,
		TargetResolution: plan.MaxResolution,
		SourceVideoCodec: BroadcastSourceCodecs.Video,
		SourceAudioCodec: BroadcastSourceCodecs.Audio,
		HWAccel:          settings.HWAccel,
		HWDecode:         settings.HWDecode,
		EncoderPreset:    settings.EncoderPreset,
		FrameRateCap:     settings.frameRateCap(),
		LeadSegments:     lead,
	})
	if err != nil {
		b.releaseTranscodeSlot(transcoding)
		_ = os.RemoveAll(dir)
		// FFmpeg reports a tuner refusal as "Server returned 5XX Server Error
		// reply" and drops the header that says why, so the raw error reaches the
		// viewer as an unactionable exit status. Ask the tuner for its own reason
		// and answer with that instead.
		if refusal := DescribeTunerRefusal(ctx, b.httpClient, req.SourceURL); refusal != nil {
			slog.WarnContext(ctx, "livetv tuner refused the channel",
				"playback_session_id", id, "error", refusal, "ffmpeg_error", err)
			return "", "", refusal
		}
		return "", "", err
	}

	b.mu.Lock()
	b.sessions[id] = &bridgeSession{
		live:               live,
		dir:                dir,
		userID:             req.UserID,
		profileID:          req.ProfileID,
		holdsTranscodeSlot: transcoding,
	}
	b.mu.Unlock()

	slog.InfoContext(ctx, "livetv hls bridge started",
		"playback_session_id", id,
		"channel_id", req.ChannelID,
		"video", plan.VideoCodec,
		"audio", plan.AudioCodec)
	return id, PublicLiveHLSPath(id), nil
}

// acquireTranscodeSlot takes an encode slot without blocking; being at capacity
// is reported to the caller rather than queued behind someone else's channel.
// The limit is read per tune so an admin can change it without a restart.
func (b *HLSBridge) acquireTranscodeSlot(maxTranscodes int) bool {
	limit := maxTranscodes
	if limit == 0 {
		limit = DefaultMaxLiveTranscodes
	}
	if limit < 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.activeTranscodes >= limit {
		return false
	}
	b.activeTranscodes++
	return true
}

func (b *HLSBridge) releaseTranscodeSlot(held bool) {
	if !held {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.activeTranscodes > 0 {
		b.activeTranscodes--
	}
}

func (b *HLSBridge) StopLiveStream(_ context.Context, playbackSessionID string) error {
	if b == nil || playbackSessionID == "" {
		return nil
	}
	b.mu.Lock()
	sess := b.sessions[playbackSessionID]
	delete(b.sessions, playbackSessionID)
	b.mu.Unlock()
	if sess == nil {
		return nil
	}
	b.releaseTranscodeSlot(sess.holdsTranscodeSlot)
	_ = sess.live.Close()
	// Delay cleanup briefly so in-flight segment fetches can finish.
	go func(dir string) {
		time.Sleep(2 * time.Second)
		_ = os.RemoveAll(dir)
	}(sess.dir)
	return nil
}

// Authorize reports whether the caller may read a live-hls session.
func (b *HLSBridge) Authorize(playbackSessionID string, userID int, profileID string, enforceOwner bool) error {
	if b == nil || playbackSessionID == "" {
		return ErrNotFound
	}
	b.mu.Lock()
	sess := b.sessions[playbackSessionID]
	b.mu.Unlock()
	if sess == nil {
		return ErrNotFound
	}
	if enforceOwner && !ownerMatches(sess.userID, sess.profileID, userID, profileID) {
		return ErrNotFound
	}
	return nil
}

// ResolvePlaylistFile returns the absolute path for a live-hls asset under a
// playback session directory. name must be a base filename (index.m3u8 or seg_*.ts).
func (b *HLSBridge) ResolvePlaylistFile(playbackSessionID, name string) (string, error) {
	if b == nil || playbackSessionID == "" || name == "" {
		return "", ErrNotFound
	}
	if filepath.Base(name) != name || name == "." || name == ".." {
		return "", ErrInvalidArgument
	}
	b.mu.Lock()
	sess := b.sessions[playbackSessionID]
	b.mu.Unlock()
	if sess == nil {
		return "", ErrNotFound
	}
	full := filepath.Join(sess.dir, name)
	if _, err := os.Stat(full); err != nil {
		return "", ErrNotFound
	}
	return full, nil
}
