package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// LiveHLSOpts configures a live HLS session. The default is a straight remux;
// setting VideoCodec or AudioCodec to a target codec re-encodes that stream for
// clients that cannot decode the broadcast format.
type LiveHLSOpts struct {
	ID         string
	InputURL   string
	OutputDir  string
	FFmpegPath string
	// SegmentSeconds is the target HLS segment duration (default 1).
	SegmentSeconds int
	// ListSize is the sliding window size (default 6).
	ListSize int
	// VideoCodec is "copy" (default) or an encode target such as "h264".
	VideoCodec string
	// AudioCodec is "copy" (default) or an encode target such as "aac".
	AudioCodec string
	// AudioChannels caps re-encoded audio; 0 keeps the stereo downmix.
	AudioChannels int
	// TargetResolution optionally scales an encoded stream down ("720p").
	TargetResolution string
	// SourceVideoCodec / SourceAudioCodec describe the broadcast streams so the
	// shared encoder helpers can apply the same source-specific rules as VOD.
	SourceVideoCodec string
	SourceAudioCodec string
	// HWAccel mirrors playback.hw_accel ("auto", "nvenc", "qsv", "vaapi",
	// "none"); "auto" probes the host exactly like the VOD transcode path.
	HWAccel string
	// HWDevice optionally pins the render device used for hardware encoding.
	HWDevice string
}

func (o LiveHLSOpts) transcodesVideo() bool {
	return o.VideoCodec != "" && !strings.EqualFold(o.VideoCodec, "copy")
}

func (o LiveHLSOpts) transcodesAudio() bool {
	return o.AudioCodec != "" && !strings.EqualFold(o.AudioCodec, "copy")
}

// buildLiveHLSArgs builds the ffmpeg command line for a live session. Copy-only
// sessions keep the low-latency remux flags; encoding sessions reuse the VOD
// encoder/filter builders so live and on-demand transcodes stay on one set of
// hardware and quality rules.
func buildLiveHLSArgs(opts LiveHLSOpts, playlist, segPattern string, segmentSeconds, listSize int) []string {
	encodeVideo := opts.transcodesVideo()
	encodeAudio := opts.transcodesAudio()

	// The VOD builders read their inputs from TranscodeOpts; borrowing the type
	// keeps encoder selection, presets, and downmix rules in one place.
	shared := TranscodeOpts{
		TargetCodecVideo:    opts.VideoCodec,
		TargetCodecAudio:    opts.AudioCodec,
		TargetAudioChannels: opts.AudioChannels,
		TargetResolution:    opts.TargetResolution,
		SourceVideoCodec:    opts.SourceVideoCodec,
		SourceAudioCodec:    opts.SourceAudioCodec,
		SegmentDuration:     segmentSeconds,
		SubtitleTrackIndex:  -1,
		FFmpegPath:          opts.FFmpegPath,
		HWAccel:             opts.HWAccel,
		HWDevice:            opts.HWDevice,
		// Live has no pre-roll to spend on encoder lookahead.
		FastStart: true,
	}
	if !encodeVideo {
		shared.TargetCodecVideo = "copy"
	}
	if !encodeAudio {
		shared.TargetCodecAudio = "copy"
	}
	shared.HWAccel = resolveEffectiveTranscodeHWAccel(shared)

	args := []string{"-hide_banner", "-loglevel", "error"}
	if encodeVideo {
		args = appendHWAccelArgs(args, shared)
	}
	args = append(args,
		"-fflags", "+genpts+nobuffer",
		"-flags", "low_delay",
		"-i", opts.InputURL,
		"-map", "0:v:0",
		"-map", "0:a:0?",
	)

	if encodeVideo {
		args = appendVideoArgs(args, shared)
	} else {
		args = append(args, "-c:v", "copy")
	}
	if encodeAudio {
		args = appendAudioArgs(args, shared)
	} else {
		args = append(args, "-c:a", "copy")
	}
	if encodeVideo {
		args = appendVideoFilterArgs(args, shared)
		args = appendSegmentBoundaryArgs(args, shared)
	}

	return append(args,
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", segmentSeconds),
		"-hls_list_size", fmt.Sprintf("%d", listSize),
		"-hls_flags", "delete_segments+omit_endlist+independent_segments+temp_file",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", segPattern,
		playlist,
	)
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
		// 1s segments get the first playlist entry out faster than the old 2s
		// default — cold tune latency is dominated by waiting for segment 0.
		seg = 1
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

	args := buildLiveHLSArgs(opts, playlist, segPattern, seg, listSize)

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
			slog.WarnContext(ctx, "live hls ffmpeg exited unexpectedly",
				"session_id", opts.ID, "error", session.err)
		}
		session.errMu.Unlock()
		close(session.done)
	}()

	// Block until the playlist lists at least one segment. Returning earlier
	// caused clients to GET index.m3u8 → 404 → hls.js manifestLoadError.
	deadline := time.Now().Add(20 * time.Second)
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
		if liveHLSPlaylistReady(playlist) {
			return session, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = session.Close()
	return nil, errors.New("live hls playlist not ready within timeout")
}

// liveHLSPlaylistReady reports whether the HLS playlist is safe for clients to
// load: non-empty and either listing a segment (#EXTINF) or accompanied by a
// non-empty .ts segment file in the same directory.
func liveHLSPlaylistReady(playlist string) bool {
	data, err := os.ReadFile(playlist)
	if err != nil || len(data) == 0 {
		return false
	}
	if bytes.Contains(data, []byte("#EXTINF")) {
		return true
	}
	dir := filepath.Dir(playlist)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".ts") {
			continue
		}
		info, infoErr := e.Info()
		if infoErr == nil && info.Size() > 0 {
			return true
		}
	}
	return false
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
