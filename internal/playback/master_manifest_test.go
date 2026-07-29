package playback

import (
	"strings"
	"testing"
)

func TestBuildMasterManifestCopySessionAdvertisesSourceCodec(t *testing.T) {
	// A copy session puts the source bytes on the wire, so that is what the
	// player must be told to expect.
	got := string(BuildMasterManifest("media.m3u8?st=tok", TranscodeOpts{
		TargetCodecVideo:  "copy",
		SourceVideoCodec:  "av1",
		TargetCodecAudio:  "aac",
		TargetResolution:  "2160p",
		TargetBitrateKbps: 20000,
	}))

	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-STREAM-INF:",
		"BANDWIDTH=20000000",
		"RESOLUTION=3840x2160",
		`CODECS="av01.0.08M.08,mp4a.40.2"`,
		"\nmedia.m3u8?st=tok\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("master playlist missing %q:\n%s", want, got)
		}
	}
}

func TestBuildMasterManifestEncodeSessionAdvertisesTargetCodec(t *testing.T) {
	got := string(BuildMasterManifest("media.m3u8", TranscodeOpts{
		TargetCodecVideo: "hevc",
		SourceVideoCodec: "av1",
		TargetCodecAudio: "aac",
		TargetResolution: "1080p",
	}))

	if !strings.Contains(got, `CODECS="hvc1.1.6.L123.b0,mp4a.40.2"`) {
		t.Errorf("encode session should advertise the target codec, not the source:\n%s", got)
	}
	if strings.Contains(got, "av01") {
		t.Errorf("encode session must not advertise the source codec:\n%s", got)
	}
	// No cap configured: BANDWIDTH is required, so a plausible default stands in.
	if !strings.Contains(got, "BANDWIDTH=8000000") {
		t.Errorf("missing default BANDWIDTH:\n%s", got)
	}
}

// A wrong attribute is worse than an absent one: RESOLUTION and CODECS are both
// optional, so anything unrecognised is omitted rather than guessed.
func TestBuildMasterManifestOmitsUnknownAttributes(t *testing.T) {
	got := string(BuildMasterManifest("media.m3u8", TranscodeOpts{
		TargetCodecVideo: "someexoticcodec",
		TargetCodecAudio: "alsoexotic",
		TargetResolution: "not-a-ladder-rung",
	}))

	if strings.Contains(got, "RESOLUTION=") {
		t.Errorf("unknown resolution should be omitted:\n%s", got)
	}
	if strings.Contains(got, "CODECS=") {
		t.Errorf("unknown codecs should be omitted entirely:\n%s", got)
	}
	// BANDWIDTH is mandatory on EXT-X-STREAM-INF, so it must survive.
	if !strings.Contains(got, "BANDWIDTH=") {
		t.Errorf("BANDWIDTH is required by the spec:\n%s", got)
	}
}

func TestBuildMasterManifestAudioOnlyStillEmitsCodecs(t *testing.T) {
	got := string(BuildMasterManifest("media.m3u8", TranscodeOpts{
		TargetCodecVideo: "unknownvideo",
		TargetCodecAudio: "ac3",
	}))
	if !strings.Contains(got, `CODECS="ac-3"`) {
		t.Errorf("a recognised audio codec should still be advertised:\n%s", got)
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
		TargetCodecAudio: "aac",
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
