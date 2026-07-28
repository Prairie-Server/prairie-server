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
	// HWDecode selects hardware decoding: "auto" (default), "on", "off".
	HWDecode string
	// EncoderPreset trades compression for encode speed; empty means
	// low latency, which is what a live source needs.
	EncoderPreset string
	// FrameRateCap optionally limits output frame rate ("30", "60"). Empty
	// keeps the broadcast frame rate, which is the goal on capable hardware.
	FrameRateCap string
	// LeadSegments is how many segments must exist before the session is
	// reported ready, giving the player buffer to absorb rate variance.
	LeadSegments int
}

// livePipeline records what the built command line actually does, so the
// effective decode path is visible in logs instead of inferred.
type livePipeline struct {
	HWAccel    string
	Decoder    string
	VideoCodec string
	AudioCodec string
	Preset     string
	FrameRate  string
}

func (p livePipeline) decodePath() string {
	if p.Decoder != "" {
		return p.Decoder
	}
	return "software"
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
func buildLiveHLSArgs(
	opts LiveHLSOpts,
	playlist, segPattern string,
	segmentSeconds, listSize int,
) ([]string, livePipeline) {
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
		EncoderPreset:       liveEncoderPreset(opts.EncoderPreset),
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

	pipeline := livePipeline{
		HWAccel:    shared.HWAccel,
		VideoCodec: shared.TargetCodecVideo,
		AudioCodec: shared.TargetCodecAudio,
	}
	if encodeVideo {
		pipeline.Preset = shared.EncoderPreset
		pipeline.Decoder = resolveLiveVideoDecoder(
			opts.HWDecode, shared.HWAccel, opts.SourceVideoCodec, opts.FFmpegPath)
		pipeline.FrameRate = liveFrameRateArg(opts.FrameRateCap)
	}

	args := []string{"-hide_banner", "-loglevel", "error"}
	if encodeVideo {
		args = appendHWAccelArgs(args, shared)
		if pipeline.Decoder != "" {
			// Input option: it selects the decoder for the stream that follows.
			args = append(args, "-c:v", pipeline.Decoder)
		}
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
		if pipeline.FrameRate != "" {
			args = append(args, "-r", pipeline.FrameRate)
		}
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
	), pipeline
}

// liveEncoderPreset defaults live sessions to low latency: a live encode that
// drops below realtime never catches up, so speed beats compression.
func liveEncoderPreset(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case EncoderPresetBalanced:
		return EncoderPresetBalanced
	case EncoderPresetQuality:
		return EncoderPresetQuality
	default:
		return EncoderPresetLowLatency
	}
}

// liveFrameRateArg converts a frame-rate cap into an ffmpeg -r value. Broadcast
// rates are fractional (59.94 / 29.97), so halving 60 has to stay fractional or
// ffmpeg resamples against a rate the source never had.
func liveFrameRateArg(cap string) string {
	switch strings.ToLower(strings.TrimSpace(cap)) {
	case "30":
		return "30000/1001"
	case "60":
		return "60000/1001"
	default:
		return ""
	}
}

// StartLiveHLS launches ffmpeg to package InputURL into a live HLS playlist,
// re-encoding the streams the client cannot decode. The caller must eventually
// Close the session to stop ffmpeg and may RemoveAll the OutputDir after Close.
//
// Hardware failures degrade instead of killing the channel: a GPU that cannot
// decode this codec drops to software decode, and a GPU that cannot be used at
// all drops to a software encode. Non-hardware failures (dead tuner, bad URL)
// fail on the first attempt so a broken channel is not retried three times.
func StartLiveHLS(parent context.Context, opts LiveHLSOpts) (*LiveHLSSession, error) {
	var lastErr error
	ladder := liveFallbackLadder(opts)
	for i, attempt := range ladder {
		session, _, err := startLiveHLSAttempt(parent, attempt)
		if err == nil {
			if i > 0 {
				slog.WarnContext(parent, "live hls started on a degraded pipeline",
					"session_id", opts.ID, "hwaccel", attempt.HWAccel, "hw_decode", attempt.HWDecode)
			}
			return session, nil
		}
		lastErr = err
		if i+1 >= len(ladder) || !looksLikeHardwareFailure(err) {
			break
		}
		slog.WarnContext(parent, "live hls hardware pipeline failed, falling back",
			"session_id", opts.ID,
			"hwaccel", attempt.HWAccel,
			"hw_decode", attempt.HWDecode,
			"error", err)
	}
	return nil, lastErr
}

// liveFallbackLadder is the ordered set of pipelines to try, most capable
// first. Copy sessions and software encodes have nothing to fall back from.
func liveFallbackLadder(opts LiveHLSOpts) []LiveHLSOpts {
	ladder := []LiveHLSOpts{opts}
	if !opts.transcodesVideo() {
		return ladder
	}
	hwAccel := ResolveHWAccelWithFFmpeg(opts.HWAccel, opts.FFmpegPath)
	if hwAccel == "" || hwAccel == "none" {
		return ladder
	}
	if !strings.EqualFold(strings.TrimSpace(opts.HWDecode), HWDecodeOff) {
		softwareDecode := opts
		softwareDecode.HWDecode = HWDecodeOff
		ladder = append(ladder, softwareDecode)
	}
	softwareAll := opts
	softwareAll.HWDecode = HWDecodeOff
	softwareAll.HWAccel = "none"
	return append(ladder, softwareAll)
}

// hardwareFailureMarkers are substrings ffmpeg prints when the GPU pipeline
// itself is the problem, as opposed to the source being unreachable.
var hardwareFailureMarkers = []string{
	"hwaccel", "hardware device", "cuda", "cuvid", "nvenc", "nvdec",
	"vaapi", "qsv", "impossible to convert between the formats",
	"error initializing a simple filtergraph",
}

func looksLikeHardwareFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range hardwareFailureMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func startLiveHLSAttempt(parent context.Context, opts LiveHLSOpts) (*LiveHLSSession, livePipeline, error) {
	session, pipeline, err := startLiveHLSOnce(parent, opts)
	if err != nil {
		// Clear partial output so a retry starts from an empty directory.
		_ = removeDirContents(opts.OutputDir)
	}
	return session, pipeline, err
}

func removeDirContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func startLiveHLSOnce(parent context.Context, opts LiveHLSOpts) (*LiveHLSSession, livePipeline, error) {
	if opts.ID == "" {
		return nil, livePipeline{}, errors.New("live hls: id required")
	}
	if opts.InputURL == "" {
		return nil, livePipeline{}, errors.New("live hls: input url required")
	}
	if opts.OutputDir == "" {
		return nil, livePipeline{}, errors.New("live hls: output dir required")
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
		return nil, livePipeline{}, fmt.Errorf("live hls mkdir: %w", err)
	}

	// The encoder must outlive the HTTP request that started it. Parent is only
	// used to bound the readiness wait; process lifetime is owned by the session.
	procCtx, cancel := context.WithCancel(context.Background())
	bin := ResolveFFmpegPath(opts.FFmpegPath)
	playlist := filepath.Join(opts.OutputDir, "index.m3u8")
	segPattern := filepath.Join(opts.OutputDir, "seg_%05d.ts")

	args, pipeline := buildLiveHLSArgs(opts, playlist, segPattern, seg, listSize)

	cmd := exec.CommandContext(procCtx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, pipeline, fmt.Errorf("live hls stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, pipeline, fmt.Errorf("live hls start: %w", err)
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
		// Keep a bounded tail rather than the last read: ffmpeg reports the
		// cause first ("Hardware device setup failed") and the consequence
		// last, and only the cause tells us whether a fallback would help.
		tail := newBoundedTailBuffer(0)
		buf := make([]byte, 4096)
		for {
			n, readErr := stderr.Read(buf)
			if n > 0 {
				_, _ = tail.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
		waitErr := cmd.Wait()
		session.errMu.Lock()
		if waitErr != nil && procCtx.Err() == nil {
			session.err = fmt.Errorf("live hls ffmpeg exited: %w (%s)", waitErr, tail.String())
			slog.WarnContext(procCtx, "live hls ffmpeg exited unexpectedly",
				"session_id", opts.ID, "error", session.err)
		}
		session.errMu.Unlock()
		close(session.done)
	}()

	// Block until the playlist lists enough segments. Returning earlier caused
	// clients to GET index.m3u8 → 404 → hls.js manifestLoadError, and starting
	// a transcode with a single segment leaves the player with no buffer to
	// absorb encoder rate variance.
	// Readiness is checked before the exit status so a run that finished
	// writing playable segments as it exited is not reported as a failure.
	lead := opts.LeadSegments
	if lead < 1 {
		lead = 1
	}
	slog.InfoContext(parent, "live hls pipeline starting",
		"session_id", opts.ID,
		"hwaccel", pipeline.HWAccel,
		"decode", pipeline.decodePath(),
		"video", pipeline.VideoCodec,
		"audio", pipeline.AudioCodec,
		"preset", pipeline.Preset,
		"frame_rate", pipeline.FrameRate,
		"lead_segments", lead)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if liveHLSSegmentCount(playlist) >= lead {
			return session, pipeline, nil
		}
		select {
		case <-parent.Done():
			_ = session.Close()
			return nil, pipeline, parent.Err()
		case <-session.done:
			if liveHLSPlaylistReady(playlist) {
				return session, pipeline, nil
			}
			session.errMu.Lock()
			err := session.err
			session.errMu.Unlock()
			if err == nil {
				err = errors.New("live hls ffmpeg exited before playlist ready")
			}
			_ = session.Close()
			return nil, pipeline, err
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = session.Close()
	return nil, pipeline, errors.New("live hls playlist not ready within timeout")
}

// liveHLSPlaylistReady reports whether the HLS playlist is safe for clients to
// load at all: it lists at least one segment.
func liveHLSPlaylistReady(playlist string) bool {
	return liveHLSSegmentCount(playlist) >= 1
}

// liveHLSSegmentCount counts the segments a client could fetch right now. It
// reads the playlist rather than the directory because a segment only becomes
// fetchable once ffmpeg has advertised it.
func liveHLSSegmentCount(playlist string) int {
	data, err := os.ReadFile(playlist)
	if err != nil || len(data) == 0 {
		return 0
	}
	if count := bytes.Count(data, []byte("#EXTINF")); count > 0 {
		return count
	}
	// Some writers flush the playlist header before the first #EXTINF; fall
	// back to segments already on disk so a ready stream is not missed.
	dir := filepath.Dir(playlist)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".ts") {
			continue
		}
		if info, infoErr := e.Info(); infoErr == nil && info.Size() > 0 {
			count++
		}
	}
	return count
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
