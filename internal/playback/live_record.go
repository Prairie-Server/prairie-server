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

// LiveRecordSession copies a live MPEG-TS HTTP source to a single output file.
type LiveRecordSession struct {
	ID         string
	OutputPath string

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}
	errMu  sync.Mutex
	err    error
}

// LiveRecordOpts configures a live recording session.
type LiveRecordOpts struct {
	ID         string
	InputURL   string
	OutputPath string
	FFmpegPath string
	// StopAt cancels the recording when reached (optional; zero = run until Close).
	StopAt time.Time
}

// StartLiveRecord launches ffmpeg to copy InputURL into OutputPath as MPEG-TS.
func StartLiveRecord(parent context.Context, opts LiveRecordOpts) (*LiveRecordSession, error) {
	if opts.ID == "" {
		return nil, errors.New("live record: id required")
	}
	if opts.InputURL == "" {
		return nil, errors.New("live record: input url required")
	}
	if opts.OutputPath == "" {
		return nil, errors.New("live record: output path required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return nil, fmt.Errorf("live record mkdir: %w", err)
	}

	ctx, cancel := context.WithCancel(parent)
	if !opts.StopAt.IsZero() {
		var stopCancel context.CancelFunc
		ctx, stopCancel = context.WithDeadline(ctx, opts.StopAt)
		prev := cancel
		cancel = func() {
			stopCancel()
			prev()
		}
	}

	bin := ResolveFFmpegPath(opts.FFmpegPath)
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-i", opts.InputURL,
		"-map", "0",
		"-c", "copy",
		"-f", "mpegts",
		opts.OutputPath,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("live record stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("live record start: %w", err)
	}

	session := &LiveRecordSession{
		ID:         opts.ID,
		OutputPath: opts.OutputPath,
		cmd:        cmd,
		cancel:     cancel,
		done:       make(chan struct{}),
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
			session.err = fmt.Errorf("live record ffmpeg exited: %w (%s)", waitErr, string(last))
			slog.WarnContext(ctx, "live record ffmpeg exited unexpectedly",
				"recording_id", opts.ID, "error", session.err)
		}
		session.errMu.Unlock()
		close(session.done)
	}()
	return session, nil
}

func (s *LiveRecordSession) Err() error {
	if s == nil {
		return nil
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *LiveRecordSession) Done() <-chan struct{} {
	if s == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return s.done
}

func (s *LiveRecordSession) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-s.done:
	case <-time.After(8 * time.Second):
		if s.cmd != nil && s.cmd.Process != nil {
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-s.done
	}
	return s.Err()
}
