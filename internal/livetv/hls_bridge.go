package livetv

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prairie-server/prairie-server/internal/idgen"
	"github.com/prairie-server/prairie-server/internal/playback"
)

// PublicLiveHLSPath is the authenticated relative URL for a live HLS remux.
func PublicLiveHLSPath(playbackSessionID string) string {
	return "/api/v1/livetv/live-hls/" + playbackSessionID + "/index.m3u8"
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
	hwAccel    string
	// transcodeSlots bounds concurrent encoding sessions; nil disables gating.
	transcodeSlots chan struct{}

	mu       sync.Mutex
	sessions map[string]*bridgeSession
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
	// as movie transcodes ("auto" probes the host).
	HWAccel string
	// MaxTranscodes bounds concurrent encoding sessions; 0 uses
	// DefaultMaxLiveTranscodes and a negative value disables the limit.
	MaxTranscodes int
}

// NewHLSBridge creates a live HLS playback bridge under Root/livetv-hls.
func NewHLSBridge(opts HLSBridgeOptions) *HLSBridge {
	root := opts.Root
	if root == "" {
		root = filepath.Join(os.TempDir(), "prairie-transcode")
	}
	maxTranscodes := opts.MaxTranscodes
	if maxTranscodes == 0 {
		maxTranscodes = DefaultMaxLiveTranscodes
	}
	bridge := &HLSBridge{
		root:       filepath.Join(root, "livetv-hls"),
		ffmpegPath: opts.FFmpegPath,
		hwAccel:    opts.HWAccel,
		sessions:   map[string]*bridgeSession{},
	}
	if maxTranscodes > 0 {
		bridge.transcodeSlots = make(chan struct{}, maxTranscodes)
	}
	return bridge
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
	plan := req.Plan
	if plan.VideoCodec == "" {
		plan.VideoCodec = "copy"
	}
	if plan.AudioCodec == "" {
		plan.AudioCodec = "copy"
	}
	transcoding := plan.Transcodes()
	if transcoding && !b.acquireTranscodeSlot() {
		return "", "", fmt.Errorf("%w: live transcode capacity reached", ErrLimitExceeded)
	}
	id, err := idgen.NextID()
	if err != nil {
		b.releaseTranscodeSlot(transcoding)
		return "", "", err
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
		HWAccel:          b.hwAccel,
	})
	if err != nil {
		b.releaseTranscodeSlot(transcoding)
		_ = os.RemoveAll(dir)
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

// acquireTranscodeSlot takes an encode slot without blocking; a full pool means
// the caller must be told the server is at capacity rather than queued behind
// someone else's channel.
func (b *HLSBridge) acquireTranscodeSlot() bool {
	if b.transcodeSlots == nil {
		return true
	}
	select {
	case b.transcodeSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (b *HLSBridge) releaseTranscodeSlot(held bool) {
	if !held || b.transcodeSlots == nil {
		return
	}
	select {
	case <-b.transcodeSlots:
	default:
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
