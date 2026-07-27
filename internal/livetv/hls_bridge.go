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

// HLSBridge remuxes HDHomeRun MPEG-TS into sliding-window HLS for Prairie clients.
type HLSBridge struct {
	root       string
	ffmpegPath string

	mu       sync.Mutex
	sessions map[string]*bridgeSession
}

type bridgeSession struct {
	live      *playback.LiveHLSSession
	dir       string
	userID    int
	profileID string
}

// NewHLSBridge creates a live HLS playback bridge under root/livetv-hls.
func NewHLSBridge(root, ffmpegPath string) *HLSBridge {
	if root == "" {
		root = filepath.Join(os.TempDir(), "prairie-transcode")
	}
	return &HLSBridge{
		root:       filepath.Join(root, "livetv-hls"),
		ffmpegPath: ffmpegPath,
		sessions:   map[string]*bridgeSession{},
	}
}

func (b *HLSBridge) StartLiveStream(
	ctx context.Context,
	channelID, sourceStreamURL string,
	userID int,
	profileID string,
) (playbackSessionID, playbackURL string, err error) {
	if b == nil {
		return "", "", fmt.Errorf("hls bridge not configured")
	}
	if err := ValidateMediaFetchURL(sourceStreamURL); err != nil {
		return "", "", err
	}
	id, err := idgen.NextID()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(b.root, id)
	live, err := playback.StartLiveHLS(ctx, playback.LiveHLSOpts{
		ID:         id,
		InputURL:   sourceStreamURL,
		OutputDir:  dir,
		FFmpegPath: b.ffmpegPath,
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", "", err
	}

	b.mu.Lock()
	b.sessions[id] = &bridgeSession{live: live, dir: dir, userID: userID, profileID: profileID}
	b.mu.Unlock()

	slog.InfoContext(ctx, "livetv hls bridge started",
		"playback_session_id", id, "channel_id", channelID)
	return id, PublicLiveHLSPath(id), nil
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
