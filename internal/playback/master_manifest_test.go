package playback

import (
	"strings"
	"testing"
)

func TestBuildMasterManifestEncodeAdvertisesLadderResolution(t *testing.T) {
	got := string(BuildMasterManifest("media.m3u8?st=tok", TranscodeOpts{
		TargetCodecVideo:  "hevc",
		TargetCodecAudio:  "aac",
		TargetResolution:  "2160p",
		TargetBitrateKbps: 20000,
	}))

	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-STREAM-INF:",
		"BANDWIDTH=20000000",
		"RESOLUTION=3840x2160",
		"\nmedia.m3u8?st=tok\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("master playlist missing %q:\n%s", want, got)
		}
	}
}

// A copy session scales nothing, so TargetResolution is empty and RESOLUTION
// must be omitted rather than restating dimensions nobody read.
func TestBuildMasterManifestCopySessionOmitsResolution(t *testing.T) {
	got := string(BuildMasterManifest("media.m3u8", TranscodeOpts{
		TargetCodecVideo: "copy",
		SourceVideoCodec: "av1",
		TargetCodecAudio: "aac",
	}))

	if strings.Contains(got, "RESOLUTION=") {
		t.Errorf("copy session should advertise no RESOLUTION:\n%s", got)
	}
	// BANDWIDTH is mandatory on EXT-X-STREAM-INF, so the default stands in.
	if !strings.Contains(got, "BANDWIDTH=8000000") {
		t.Errorf("missing default BANDWIDTH:\n%s", got)
	}
}

// RFC 6381 identifiers pin profile, tier, level and bit depth. The session
// carries only a codec family, so emitting one would be a guess a strict client
// is entitled to believe — and a mismatched decoder configuration is worse than
// none. This must stay absent until the probed stream parameters are threaded
// through and exact values can be derived.
func TestBuildMasterManifestNeverGuessesCodecs(t *testing.T) {
	for _, opts := range []TranscodeOpts{
		{TargetCodecVideo: "copy", SourceVideoCodec: "av1", TargetCodecAudio: "aac"},
		{TargetCodecVideo: "hevc", TargetCodecAudio: "aac", TargetResolution: "1080p"},
		{TargetCodecVideo: "h264", TargetCodecAudio: "ac3", TargetResolution: "720p"},
	} {
		got := string(BuildMasterManifest("media.m3u8", opts))
		if strings.Contains(got, "CODECS=") {
			t.Errorf("CODECS advertised without probed stream parameters (%+v):\n%s", opts, got)
		}
		for _, invented := range []string{"avc1.", "hvc1.", "av01.", "mp4a."} {
			if strings.Contains(got, invented) {
				t.Errorf("invented codec identifier %q present (%+v):\n%s", invented, opts, got)
			}
		}
	}
}

func TestBuildMasterManifestOmitsUnknownResolution(t *testing.T) {
	got := string(BuildMasterManifest("media.m3u8", TranscodeOpts{
		TargetResolution: "not-a-ladder-rung",
	}))
	if strings.Contains(got, "RESOLUTION=") {
		t.Errorf("unknown resolution should be omitted:\n%s", got)
	}
	if !strings.Contains(got, "BANDWIDTH=") {
		t.Errorf("BANDWIDTH is required by the spec:\n%s", got)
	}
}

// The media URI must survive verbatim: the signed stream token rides its query
// string, and a player that drops or re-encodes it cannot fetch the playlist.
func TestMasterManifestMediaURIPreservesQuery(t *testing.T) {
	if got := MasterManifestMediaURI("st=abc.def-ghi&token=xyz"); got != "media.m3u8?st=abc.def-ghi&token=xyz" {
		t.Errorf("MasterManifestMediaURI() = %q", got)
	}
	if got := MasterManifestMediaURI(""); got != "media.m3u8" {
		t.Errorf("MasterManifestMediaURI(\"\") = %q, want a bare relative path", got)
	}
}

// The master playlist is only useful if it parses as one: exactly one
// EXT-X-STREAM-INF, immediately followed by the URI line.
func TestBuildMasterManifestShape(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(string(BuildMasterManifest("media.m3u8?st=t", TranscodeOpts{
		TargetCodecVideo: "hevc",
		TargetResolution: "1080p",
	}))), "\n")

	if lines[0] != "#EXTM3U" {
		t.Fatalf("first line = %q, want #EXTM3U", lines[0])
	}
	streamInf := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			if streamInf >= 0 {
				t.Fatalf("more than one EXT-X-STREAM-INF:\n%s", strings.Join(lines, "\n"))
			}
			streamInf = i
		}
	}
	if streamInf < 0 {
		t.Fatalf("no EXT-X-STREAM-INF:\n%s", strings.Join(lines, "\n"))
	}
	if streamInf != len(lines)-2 || lines[len(lines)-1] != "media.m3u8?st=t" {
		t.Fatalf("EXT-X-STREAM-INF must be immediately followed by the URI:\n%s", strings.Join(lines, "\n"))
	}
}
