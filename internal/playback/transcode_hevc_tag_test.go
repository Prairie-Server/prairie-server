package playback

import (
	"strings"
	"testing"
)

func copyHEVCOpts() TranscodeOpts {
	return TranscodeOpts{
		InputPath:        "/mnt/media/movie.mkv",
		OutputDir:        "/tmp/out",
		SourceVideoCodec: "hevc",
		TargetCodecVideo: "copy",
		TargetCodecAudio: "aac",
		SegmentDuration:  2,
		TotalDuration:    600,
	}
}

// hev1 and hvc1 differ only in whether parameter sets may live in-band, but
// Samsung's Tizen pipeline and Apple's both reject hev1 in fMP4 -- surfacing to
// the app as an unsupported format only after init.mp4 has been fetched, which
// is exactly how this presented on a retail TV.
func TestCopiedHEVCInFMP4IsTaggedHvc1(t *testing.T) {
	args := buildFFmpegArgs(copyHEVCOpts())

	if !argsContainPair(args, "-tag:v", "hvc1") {
		t.Fatalf("copied HEVC in fMP4 must be tagged hvc1, args=%v", strings.Join(args, " "))
	}
	// Guard the premise: if this stopped being an fMP4 session the tag would be
	// wrong rather than merely unnecessary.
	if !argsContainPair(args, "-hls_segment_type", "fmp4") {
		t.Fatalf("expected an fMP4 session, args=%v", strings.Join(args, " "))
	}
	if !argsContainPair(args, "-c:v", "copy") {
		t.Fatalf("expected copied video, args=%v", strings.Join(args, " "))
	}
}

// MPEG-TS carries HEVC in-band by design and has no sample entry, so a tag
// there would be meaningless. MPEG-2 sources are the codec-copy path that stays
// on TS for Apple compatibility HLS.
func TestCopiedHEVCInMPEGTSIsNotTagged(t *testing.T) {
	opts := copyHEVCOpts()
	opts.SourceVideoCodec = "mpeg2video"

	args := buildFFmpegArgs(opts)
	if argsContainPair(args, "-hls_segment_type", "fmp4") {
		t.Fatalf("MPEG-2 copy should stay on MPEG-TS, args=%v", strings.Join(args, " "))
	}
	if argsContainPair(args, "-tag:v", "hvc1") {
		t.Errorf("MPEG-TS session must not carry an fMP4 sample-entry tag, args=%v", strings.Join(args, " "))
	}
}

// The tag describes a copied HEVC bitstream. An encode produces its own sample
// entry from the encoder, and tagging a non-HEVC codec hvc1 would mislabel it.
func TestHvc1TagOnlyForCopiedHEVC(t *testing.T) {
	for _, tc := range []struct {
		name             string
		sourceVideoCodec string
		targetCodecVideo string
	}{
		{name: "h264 copy", sourceVideoCodec: "h264", targetCodecVideo: "copy"},
		{name: "av1 copy", sourceVideoCodec: "av1", targetCodecVideo: "copy"},
		{name: "hevc encode", sourceVideoCodec: "hevc", targetCodecVideo: "hevc"},
		{name: "hevc source encoded to h264", sourceVideoCodec: "hevc", targetCodecVideo: "h264"},
	} {
		opts := copyHEVCOpts()
		opts.SourceVideoCodec = tc.sourceVideoCodec
		opts.TargetCodecVideo = tc.targetCodecVideo
		if tc.targetCodecVideo != "copy" {
			opts.TargetResolution = resolution1080p
		}

		args := buildFFmpegArgs(opts)
		if argsContainPair(args, "-tag:v", "hvc1") {
			t.Errorf("%s must not be tagged hvc1, args=%v", tc.name, strings.Join(args, " "))
		}
	}
}

// A Dolby Vision source reaching the HLS copy path has had its RPU stripped to
// plain HDR10, so the stream leaving the muxer really is ordinary HEVC and hvc1
// is the correct label for it. The bitstream filter must survive alongside.
func TestDolbyVisionStrippedCopyStillTaggedHvc1(t *testing.T) {
	opts := copyHEVCOpts()
	opts.VideoBitstreamFilter = DV7ToHDR10BitstreamFilter

	args := buildFFmpegArgs(opts)
	if !argsContainPair(args, "-bsf:v", DV7ToHDR10BitstreamFilter) {
		t.Fatalf("DV->HDR10 bitstream filter was dropped, args=%v", strings.Join(args, " "))
	}
	if !argsContainPair(args, "-tag:v", "hvc1") {
		t.Errorf("stripped DV copy is plain HEVC and must be tagged hvc1, args=%v", strings.Join(args, " "))
	}
}

func TestIsHEVCVideoCodecAliases(t *testing.T) {
	for _, alias := range []string{"hevc", "HEVC", "h265", "H.265", "h-265", "x265", "hev1", "hvc1", " hevc "} {
		if !IsHEVCVideoCodec(alias) {
			t.Errorf("IsHEVCVideoCodec(%q) = false, want true", alias)
		}
	}
	for _, other := range []string{"", "h264", "avc1", "av1", "av01", "vp9", "mpeg2video"} {
		if IsHEVCVideoCodec(other) {
			t.Errorf("IsHEVCVideoCodec(%q) = true, want false", other)
		}
	}
}

// E-AC-3 is fine to copy -- a source E-AC-3 track passes through via the copy
// path, which is the direct-play route -- but it must never be an encode target:
// Tizen accepts E-AC-3 for direct play and not for transcode, and AC-3 has
// broader hardware decode support at the same layout.
func TestEAC3IsNeverAnEncodeTarget(t *testing.T) {
	opts := copyHEVCOpts()
	opts.TargetCodecAudio = "eac3"
	opts.SourceAudioCodec = "truehd"
	opts.SourceAudioChannels = 8

	args := buildFFmpegArgs(opts)
	if argsContainPair(args, "-c:a", "eac3") {
		t.Errorf("E-AC-3 was selected as an encode target, args=%v", strings.Join(args, " "))
	}
	if !argsContainPair(args, "-c:a", "ac3") {
		t.Errorf("expected an AC-3 fallback, args=%v", strings.Join(args, " "))
	}
	// The fixture is 8-channel and the AC-3 encoder tops out at 5.1, so the
	// fallback has to state a layout it can actually encode. Asserting the codec
	// alone would pass while emitting an unencodable 7.1 request.
	if !argsContainPair(args, "-ac", "6") {
		t.Errorf("8-channel source must be capped at 5.1 for AC-3, args=%v", strings.Join(args, " "))
	}
	if !argsContainPair(args, "-b:a", "448k") {
		t.Errorf("5.1 AC-3 should pair with the surround bitrate, args=%v", strings.Join(args, " "))
	}
}

// The AC-3 target is the surround-preserving route, so it must not inherit the
// AAC path's fragile-lossless stereo downmix: a TrueHD source is exactly the
// case where a client asked for AC-3 to keep 5.1 for its receiver.
func TestAC3TargetKeepsSurroundForTrueHDSource(t *testing.T) {
	opts := copyHEVCOpts()
	opts.TargetCodecAudio = "ac3"
	opts.SourceAudioCodec = "truehd"
	opts.SourceAudioChannels = 8
	opts.TargetAudioChannels = 6

	args := buildFFmpegArgs(opts)
	if argsContainPair(args, "-ac", "2") {
		t.Errorf("AC-3 target must not downmix TrueHD to stereo, args=%v", strings.Join(args, " "))
	}
	if !argsContainPair(args, "-ac", "6") {
		t.Errorf("want 5.1, args=%v", strings.Join(args, " "))
	}
}

// A client that declared a stereo ceiling must not be handed 5.1, and the
// bitrate should follow the layout down rather than padding the stream.
func TestAC3TargetHonorsClientChannelCeiling(t *testing.T) {
	opts := copyHEVCOpts()
	opts.TargetCodecAudio = "ac3"
	opts.SourceAudioCodec = "dts"
	opts.SourceAudioChannels = 6
	opts.TargetAudioChannels = 2

	args := buildFFmpegArgs(opts)
	if !argsContainPair(args, "-ac", "2") {
		t.Errorf("declared stereo ceiling must cap the AC-3 layout, args=%v", strings.Join(args, " "))
	}
	if !argsContainPair(args, "-b:a", "192k") {
		t.Errorf("stereo AC-3 should not carry the 5.1 bitrate, args=%v", strings.Join(args, " "))
	}
}

// With no probed source count and no binding client ceiling there is nothing to
// choose from, so the layout is left to FFmpeg rather than guessed at -- naming
// 5.1 here would upmix a source that turns out to be stereo.
func TestAC3TargetLeavesUnknownLayoutToFFmpeg(t *testing.T) {
	opts := copyHEVCOpts()
	opts.TargetCodecAudio = "ac3"
	opts.SourceAudioCodec = "dts"
	opts.SourceAudioChannels = 0
	opts.TargetAudioChannels = 0

	args := buildFFmpegArgs(opts)
	for _, n := range []string{"1", "2", "6", "8"} {
		if argsContainPair(args, "-ac", n) {
			t.Errorf("unknown layout must not be pinned to %s channels, args=%v", n, strings.Join(args, " "))
		}
	}
	if !argsContainPair(args, "-c:a", "ac3") {
		t.Errorf("expected an AC-3 encode, args=%v", strings.Join(args, " "))
	}
}

// Copying a source E-AC-3 track is unaffected -- that is the direct-play case
// Tizen does support.
func TestEAC3SourceStillCopies(t *testing.T) {
	opts := copyHEVCOpts()
	opts.TargetCodecAudio = "copy"
	opts.SourceAudioCodec = "eac3"

	args := buildFFmpegArgs(opts)
	if !argsContainPair(args, "-c:a", "copy") {
		t.Errorf("a copy target must still copy, args=%v", strings.Join(args, " "))
	}
}
