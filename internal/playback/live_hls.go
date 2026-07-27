package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// LiveHLSSession is an ffmpeg remux of a live MPEG-TS HTTP source into a
// sliding-window HLS playlist under OutputDir.
type LiveHLSSession struct {
	ID        string
	OutputDir string
	Playlist  string // absolute path to index.m3u8

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}
	errMu  sync.Mutex
	err    error
}

// LiveHLSOpts configures a live HLS remux session.
type LiveHLSOpts struct {
	ID         string
	InputURL   string
	OutputDir  string
	FFmpegPath string
	// SegmentSeconds is the target HLS segment duration (default 2).
	SegmentSeconds int
	// ListSize is the sliding window size (default 6).
	ListSize int
}

// StartLiveHLS launches ffmpeg to remux InputURL into a live HLS playlist.
// The caller must eventually Close the session to stop ffmpeg and may RemoveAll
// the OutputDir after Close.
func StartLiveHLS(parent context.Context, opts LiveHLSOpts) (*LiveHLSSession, error) {
	if opts.ID == "" {
		return nil, errors.New("live hls: id required")
	}
	if opts.InputURL == "" {
		return nil, errors.New("live hls: input url required")
	}
	if opts.OutputDir == "" {
		return nil, errors.New("live hls: output dir required")
	}
	seg := opts.SegmentSeconds
	if seg <= 0 {
		seg = 2
	}
	listSize := opts.ListSize
	if listSize <= 0 {
		listSize = 6
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("live hls mkdir: %w", err)
	}

	ctx, cancel := context.WithCancel(parent)
	bin := ResolveFFmpegPath(opts.FFmpegPath)
	playlist := filepath.Join(opts.OutputDir, "index.m3u8")
	segPattern := filepath.Join(opts.OutputDir, "seg_%05d.ts")

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "+genpts+nobuffer",
		"-flags", "low_delay",
		"-i", opts.InputURL,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c", "copy",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", seg),
		"-hls_list_size", fmt.Sprintf("%d", listSize),
		"-hls_flags", "delete_segments+omit_endlist+independent_segments+temp_file",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", segPattern,
		playlist,
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("live hls stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("live hls start: %w", err)
	}

	session := &LiveHLSSession{
		ID:        opts.ID,
		OutputDir: opts.OutputDir,
		Playlist:  playlist,
		cmd:       cmd,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	go func() {
		buf := make([]byte, 4096)
		var last []byte
		for {
			n, readErr := stderr.Read(buf)
			if n > 0 {
				last = append(last[:0], buf[:n]...)
			}
			if readErr != nil {
				break
			}
		}
		waitErr := cmd.Wait()
		session.errMu.Lock()
		if waitErr != nil && ctx.Err() == nil {
			session.err = fmt.Errorf("live hls ffmpeg exited: %w (%s)", waitErr, string(last))
			slog.Warn("live hls ffmpeg exited unexpectedly",
				"session_id", opts.ID, "error", session.err)
		}
		session.errMu.Unlock()
		close(session.done)
	}()

	// Wait briefly for the playlist to appear so clients don't 404 immediately.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-session.done:
			session.errMu.Lock()
			err := session.err
			session.errMu.Unlock()
			if err == nil {
				err = errors.New("live hls ffmpeg exited before playlist ready")
			}
			_ = session.Close()
			return nil, err
		default:
		}
		if st, err := os.Stat(playlist); err == nil && st.Size() > 0 {
			return session, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Soft-ready: process is still running; playlist may appear on first segments.
	return session, nil
}

// Err returns a non-nil error if ffmpeg exited unexpectedly.
func (s *LiveHLSSession) Err() error {
	if s == nil {
		return nil
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

// Done is closed when the ffmpeg process exits.
func (s *LiveHLSSession) Done() <-chan struct{} {
	if s == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return s.done
}

// Close stops ffmpeg. It does not remove OutputDir.
func (s *LiveHLSSession) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		// Kill the process group in case ffmpeg spawned helpers.
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		if s.cmd != nil && s.cmd.Process != nil {
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-s.done
	}
	return s.Err()
}
