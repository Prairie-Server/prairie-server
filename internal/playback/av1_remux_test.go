package playback

import (
	"strings"
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

// AV1-capable clients should get the original video stream copied rather than a
// full re-encode: AV1 sources are usually 4K HDR, and re-encoding them to h264
// costs an NVENC pass and throws away quality for no benefit.
func TestResolve_AV1SourceOnAV1ClientRemuxesInsteadOfTranscoding(t *testing.T) {
	t.Parallel()

	file := &models.MediaFile{
		CodecVideo: "av1",
		CodecAudio: "truehd",
		Container:  "mkv",
		Resolution: "2160p",
	}
	caps := ClientCapabilities{
		CodecsVideo:   []string{"h264", "hevc", "av1"},
		CodecsAudio:   []string{"aac", "ac3", "eac3", "mp3"},
		Containers:    []string{"mp4", "mpegts", "hls"},
		MaxResolution: "2160p",
	}

	decision := Resolve(file, caps, AdminSettings{TranscodeEnabled: true, Allow4KTranscode: true})

	if decision.Method != PlayRemux {
		t.Fatalf("method = %q, want %q (video copy, audio transcode)", decision.Method, PlayRemux)
	}
	if !decision.TranscodeAudio {
		t.Fatalf("TranscodeAudio = false, want true — TrueHD needs re-encoding")
	}
}

func TestResolve_AV1SourceWithoutAV1SupportStillTranscodes(t *testing.T) {
	t.Parallel()

	file := &models.MediaFile{
		CodecVideo: "av1",
		CodecAudio: "aac",
		Container:  "mp4",
		Resolution: "1080p",
	}
	caps := ClientCapabilities{
		CodecsVideo:   []string{"h264", "hevc"},
		CodecsAudio:   []string{"aac"},
		Containers:    []string{"mp4", "hls"},
		MaxResolution: "2160p",
	}

	decision := Resolve(file, caps, AdminSettings{TranscodeEnabled: true})
	if decision.Method != PlayTranscode {
		t.Fatalf("method = %q, want %q", decision.Method, PlayTranscode)
	}
}

func TestResolve_AV1DirectPlaysWhenContainerAndAudioAlsoMatch(t *testing.T) {
	t.Parallel()

	file := &models.MediaFile{
		CodecVideo: "av1",
		CodecAudio: "aac",
		Container:  "mp4",
		Resolution: "2160p",
	}
	caps := ClientCapabilities{
		CodecsVideo:   []string{"h264", "av1"},
		CodecsAudio:   []string{"aac"},
		Containers:    []string{"mp4"},
		MaxResolution: "2160p",
	}

	decision := Resolve(file, caps, AdminSettings{TranscodeEnabled: true})
	if decision.Method != PlayDirect {
		t.Fatalf("method = %q, want %q", decision.Method, PlayDirect)
	}
}

// MPEG-TS cannot carry AV1, so an AV1 copy session has to package CMAF/fMP4.
func TestBuildFFmpegArgs_AV1CopyUsesFMP4Segments(t *testing.T) {
	t.Parallel()

	args := buildFFmpegArgs(TranscodeOpts{
		InputPath:        "/media/movie.mkv",
		OutputDir:        "/tmp/out",
		SessionID:        "session-av1-copy",
		SourceVideoCodec: "av1",
		TargetCodecVideo: "copy",
		TargetCodecAudio: "aac",
		SegmentDuration:  4,
	})
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "-c:v copy") {
		t.Fatalf("av1 session should copy video: %s", joined)
	}
	if !strings.Contains(joined, "-hls_segment_type fmp4") {
		t.Fatalf("av1 copy must use fMP4 segments, MPEG-TS cannot carry AV1: %s", joined)
	}
	if !strings.Contains(joined, "seg_%05d.m4s") {
		t.Fatalf("av1 copy should write .m4s segments: %s", joined)
	}
	if strings.Contains(joined, "-hls_segment_type mpegts") {
		t.Fatalf("av1 copy must not fall back to MPEG-TS: %s", joined)
	}
	// Audio still re-encodes; only the video pass is skipped.
	if !strings.Contains(joined, "-c:a aac") {
		t.Fatalf("av1 copy should transcode audio: %s", joined)
	}
	for _, encoder := range []string{"libx264", "h264_nvenc", "h264_qsv", "h264_vaapi", "libsvtav1"} {
		if strings.Contains(joined, encoder) {
			t.Fatalf("av1 copy should not invoke encoder %s: %s", encoder, joined)
		}
	}
}

func TestBuildFFmpegArgs_AV1CopyKeepsFragDiscontForFMP4(t *testing.T) {
	t.Parallel()

	args := buildFFmpegArgs(TranscodeOpts{
		InputPath:        "/media/movie.mkv",
		OutputDir:        "/tmp/out",
		SessionID:        "session-av1-frag",
		SourceVideoCodec: "av1",
		TargetCodecVideo: "copy",
		TargetCodecAudio: "aac",
		SegmentDuration:  4,
	})
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "movflags=+frag_discont") {
		t.Fatalf("fMP4 segments need frag_discont for A/V sync: %s", joined)
	}
}
