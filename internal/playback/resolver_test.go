package playback_test

import (
	"errors"
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/playback"
)

func defaultCaps() playback.ClientCapabilities {
	return playback.ClientCapabilities{
		CodecsVideo:   []string{"h264"},
		CodecsAudio:   []string{"aac", "opus"},
		Containers:    []string{"mp4", "webm"},
		MaxResolution: "1080p",
		HDR:           false,
	}
}

func defaultSettings() playback.AdminSettings {
	return playback.AdminSettings{
		TranscodeEnabled: true,
		Allow4KTranscode: false,
	}
}

func TestResolver_DirectPlay(t *testing.T) {
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "aac", Container: "mp4",
		Resolution: "1080p", HDR: false,
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayDirect {
		t.Errorf("method = %q, want direct", decision.Method)
	}
}

func TestResolver_Remux(t *testing.T) {
	// h264+aac in mkv — client supports codecs but not container.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "aac", Container: "mkv",
		Resolution: "1080p", HDR: false,
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayRemux {
		t.Errorf("method = %q, want remux", decision.Method)
	}
}

func TestResolver_RemuxWithAudioTranscode(t *testing.T) {
	// h264 video (supported) + dts audio (unsupported) → remux with audio transcode.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "dts", Container: "mkv",
		Resolution: "1080p", HDR: false,
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayRemux {
		t.Errorf("method = %q, want remux", decision.Method)
	}
	if !decision.TranscodeAudio {
		t.Error("TranscodeAudio = false, want true")
	}
}

func TestResolver_CopyUnsafeForcesTranscode(t *testing.T) {
	unsafe := true
	// h264+dts in mkv would normally remux with audio transcode (video copied),
	// but the source carries conflicting in-band PPS, so the video copy is unsafe
	// and it must fall through to a full transcode.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "dts", Container: "mkv",
		Resolution: "1080p", HDR: false,
		VideoTracks: []models.VideoTrack{{Codec: "h264", MultiplePPS: &unsafe}},
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayTranscode {
		t.Errorf("method = %q, want transcode", decision.Method)
	}
}

func TestResolver_UnknownCopySafetyForcesTranscode(t *testing.T) {
	// An inconclusive safety scan must not fail open to video stream-copy.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "dts", Container: "mkv",
		Resolution: "1080p", HDR: false,
		VideoTracks: []models.VideoTrack{{
			Codec:           "h264",
			VideoCopyUnsafe: true,
		}},
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayTranscode {
		t.Errorf("method = %q, want transcode", decision.Method)
	}
}

func TestResolver_CopySafeStillRemuxes(t *testing.T) {
	safe := false
	// The same shape with the copy-safety scan resolved to safe keeps remuxing.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "dts", Container: "mkv",
		Resolution: "1080p", HDR: false,
		VideoTracks: []models.VideoTrack{{Codec: "h264", MultiplePPS: &safe}},
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayRemux {
		t.Errorf("method = %q, want remux", decision.Method)
	}
}

func TestResolver_AudioPassthroughSkipsAudioTranscode(t *testing.T) {
	// Source is h264 + eac3 in mp4. Client can decode h264 but not eac3; its
	// sink advertises eac3 passthrough (e.g. HDMI AVR). Should direct-play
	// without audio transcode instead of promoting to remux.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "eac3", Container: "mp4",
		Resolution: "1080p", HDR: false,
	}
	caps := defaultCaps()
	caps.AudioPassthroughCodecs = []string{"eac3", "ac3"}

	decision := playback.Resolve(file, caps, defaultSettings())

	if decision.Method != playback.PlayDirect {
		t.Errorf("method = %q, want direct (passthrough-supported audio)", decision.Method)
	}
	if decision.TranscodeAudio {
		t.Error("TranscodeAudio = true, want false (sink can passthrough)")
	}
}

func TestResolver_AudioPassthroughAllowsContainerRemux(t *testing.T) {
	// Source is h264 + eac3 in mkv. Client passthrough covers eac3 but container
	// is unsupported → remux without audio transcode.
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "eac3", Container: "mkv",
		Resolution: "1080p", HDR: false,
	}
	caps := defaultCaps()
	caps.AudioPassthroughCodecs = []string{"eac3"}

	decision := playback.Resolve(file, caps, defaultSettings())

	if decision.Method != playback.PlayRemux {
		t.Errorf("method = %q, want remux", decision.Method)
	}
	if decision.TranscodeAudio {
		t.Error("TranscodeAudio = true, want false (sink can passthrough)")
	}
}

func TestResolver_Transcode_UnsupportedVideoCodec(t *testing.T) {
	// hevc is not in client's supported codecs.
	file := &models.MediaFile{
		CodecVideo: "hevc", CodecAudio: "aac", Container: "mp4",
		Resolution: "1080p", HDR: false,
	}
	decision := playback.Resolve(file, defaultCaps(), defaultSettings())

	if decision.Method != playback.PlayTranscode {
		t.Errorf("method = %q, want transcode", decision.Method)
	}
}

func TestResolver_Transcode_ResolutionExceeds(t *testing.T) {
	file := &models.MediaFile{
		CodecVideo: "h264", CodecAudio: "aac", Container: "mp4",
		Resolution: "2160p", HDR: false,
	}
	caps := defaultCaps()
	caps.MaxResolution = "1080p"

	decision := playback.Resolve(file, caps, defaultSettings())

	if decision.Method != playback.PlayTranscode {
		t.Errorf("method = %q, want transcode for resolution downscale", decision.Method)
	}
}

func TestResolver_HDR_PassthroughToRemux(t *testing.T) {
	file := &models.MediaFile{
		CodecVideo: "hevc", CodecAudio: "aac", Container: "mkv",
		Resolution: "1080p", HDR: true,
	}
	caps := defaultCaps()
	caps.CodecsVideo = []string{"h264", "hevc"}
	caps.HDR = false

	decision := playback.Resolve(file, caps, defaultSettings())

	if decision.Method != playback.PlayRemux {
		t.Errorf("method = %q, want remux — HDR should pass through without tone mapping", decision.Method)
	}
}

func TestResolver_TranscodeDisabled_FallsToDirect(t *testing.T) {
	file := &models.MediaFile{
		CodecVideo: "hevc", CodecAudio: "aac", Container: "mkv",
		Resolution: "1080p", HDR: false,
	}
	settings := defaultSettings()
	settings.TranscodeEnabled = false

	decision := playback.Resolve(file, defaultCaps(), settings)

	if decision.Method != playback.PlayDirect {
		t.Errorf("method = %q, want direct (transcode disabled)", decision.Method)
	}
}

func TestSelectVersion_PrefersDirectPlay(t *testing.T) {
	files := []*models.MediaFile{
		{ID: 1, CodecVideo: "hevc", CodecAudio: "truehd", Container: "mkv", Resolution: "2160p", HDR: true, FileSize: 40000000000},
		{ID: 2, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "1080p", HDR: false, FileSize: 5000000000},
	}

	decision, err := playback.SelectVersion(files, defaultCaps(), defaultSettings())
	if err != nil {
		t.Fatalf("SelectVersion: %v", err)
	}

	if decision.Method != playback.PlayDirect {
		t.Errorf("method = %q, want direct", decision.Method)
	}
	if decision.File.ID != 2 {
		t.Errorf("file ID = %d, want 2 (1080p h264 is directly playable)", decision.File.ID)
	}
}

func TestSelectVersion_PrefersHigherQuality(t *testing.T) {
	files := []*models.MediaFile{
		{ID: 1, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "720p", HDR: false, FileSize: 2000000000},
		{ID: 2, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "1080p", HDR: false, FileSize: 5000000000},
	}

	decision, err := playback.SelectVersion(files, defaultCaps(), defaultSettings())
	if err != nil {
		t.Fatalf("SelectVersion: %v", err)
	}

	if decision.File.ID != 2 {
		t.Errorf("file ID = %d, want 2 (1080p > 720p)", decision.File.ID)
	}
}

func TestSelectVersion_4KTranscodeDisabled(t *testing.T) {
	// Only a 4K file available, client max is 1080p, 4K transcode disabled.
	files := []*models.MediaFile{
		{ID: 1, CodecVideo: "hevc", CodecAudio: "truehd", Container: "mkv", Resolution: "2160p", HDR: true, FileSize: 40000000000},
	}
	caps := defaultCaps()
	caps.MaxResolution = "1080p"

	settings := defaultSettings()
	settings.Allow4KTranscode = false

	decision, err := playback.SelectVersion(files, caps, settings)
	if err != nil {
		t.Fatalf("SelectVersion: %v", err)
	}

	// Should fall back to the only file available.
	if decision.File.ID != 1 {
		t.Errorf("file ID = %d, want 1 (only file)", decision.File.ID)
	}
}

func TestSelectVersion_NoFiles(t *testing.T) {
	_, err := playback.SelectVersion(nil, defaultCaps(), defaultSettings())
	if !errors.Is(err, playback.ErrNoVersions) {
		t.Errorf("err = %v, want ErrNoVersions", err)
	}
}

func TestSelectVersion_SmallerFileBreaksTie(t *testing.T) {
	files := []*models.MediaFile{
		{ID: 1, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "1080p", HDR: false, FileSize: 8000000000},
		{ID: 2, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "1080p", HDR: false, FileSize: 5000000000},
	}

	decision, err := playback.SelectVersion(files, defaultCaps(), defaultSettings())
	if err != nil {
		t.Fatalf("SelectVersion: %v", err)
	}

	if decision.File.ID != 2 {
		t.Errorf("file ID = %d, want 2 (smaller file at same resolution)", decision.File.ID)
	}
}

func TestSelectVersionFiltered_StaysWithinEditionAndPresentation(t *testing.T) {
	files := []*models.MediaFile{
		{
			ID: 1, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "1080p",
			EditionKey: "theatrical", PresentationKind: "multipart_movie", PresentationGroupKey: "movie",
		},
		{
			ID: 2, CodecVideo: "h264", CodecAudio: "aac", Container: "mp4", Resolution: "2160p",
			EditionKey: "extended", PresentationKind: "single",
		},
	}

	decision, err := playback.SelectVersionFiltered(
		files,
		defaultCaps(),
		defaultSettings(),
		playback.VersionSelectionFilter{
			EditionKey:           "theatrical",
			PresentationKind:     "multipart_movie",
			PresentationGroupKey: "movie",
		},
	)
	if err != nil {
		t.Fatalf("SelectVersionFiltered: %v", err)
	}
	if decision.File.ID != 1 {
		t.Fatalf("file ID = %d, want 1", decision.File.ID)
	}
}

// A remux rewrites the container under the client, so a flat codecs_audio claim
// ("I can decode ac3") is not the same claim as "I can decode ac3 in MP4".
// Conflating them produced a 4K AV1 stream that played video with no sound: the
// TV listed ac3 from an unprobed static default, the source had an AC3 companion
// track, the audio was copied into MP4, and the TV reported "not supported audio
// codec but video can be played".
func TestResolveRemuxConvertsAudioNotSafeInMP4(t *testing.T) {
	settings := playback.AdminSettings{TranscodeEnabled: true}
	caps := playback.ClientCapabilities{
		CodecsVideo:   []string{"av1", "hevc", "h264"},
		CodecsAudio:   []string{"aac", "ac3", "eac3", "mp3"},
		Containers:    []string{"mp4"},
		MaxResolution: "2160p",
	}

	for _, tc := range []struct {
		codec        string
		wantTranArg  bool
		wanteDescrip string
	}{
		// AC-3/E-AC-3 copy on a plain claim: Samsung reports AC-3 on all models
		// and E-AC-3 on 2018+, and Apple platforms handle both, so requiring sink
		// passthrough only forced needless re-encodes.
		{codec: "ac3", wantTranArg: false, wanteDescrip: "AC-3 decodes from MP4 wherever the codec does"},
		{codec: "eac3", wantTranArg: false, wanteDescrip: "E-AC-3 likewise"},
		// DTS and TrueHD stay evidence-gated: Tizen reports no DTS decode, and
		// TrueHD in MP4 is unsupported essentially everywhere.
		{codec: "dts", wantTranArg: true, wanteDescrip: "DTS is fragile in MP4 without sink evidence"},
		{codec: "truehd", wantTranArg: true, wanteDescrip: "TrueHD likewise"},
		{codec: "mp3", wantTranArg: true, wanteDescrip: "MP3 in MP4 is marginal"},
		{codec: "aac", wantTranArg: false, wanteDescrip: "AAC is MP4's native audio codec"},
	} {
		file := &models.MediaFile{
			CodecVideo: "av1", CodecAudio: tc.codec, Container: "mkv", Resolution: "2160p",
		}
		decision := playback.Resolve(file, caps, settings)
		if decision.Method != playback.PlayRemux {
			t.Errorf("%s: Method = %q, want remux", tc.codec, decision.Method)
			continue
		}
		if decision.TranscodeAudio != tc.wantTranArg {
			t.Errorf("%s: TranscodeAudio = %v, want %v (%s)",
				tc.codec, decision.TranscodeAudio, tc.wantTranArg, tc.wanteDescrip)
		}
	}
}

// A declared sink passthrough is evidence about delivery, not merely about a
// decoder existing somewhere, so surround audio is still stream-copied to an AVR
// instead of being downmixed to stereo AAC.
func TestResolveRemuxKeepsPassthroughAudio(t *testing.T) {
	settings := playback.AdminSettings{TranscodeEnabled: true}
	caps := playback.ClientCapabilities{
		CodecsVideo:            []string{"hevc"},
		CodecsAudio:            []string{"aac"},
		AudioPassthroughCodecs: []string{"eac3", "truehd"},
		Containers:             []string{"mp4"},
		MaxResolution:          "2160p",
	}

	for _, codec := range []string{"eac3", "truehd"} {
		file := &models.MediaFile{
			CodecVideo: "hevc", CodecAudio: codec, Container: "mkv", Resolution: "2160p",
		}
		decision := playback.Resolve(file, caps, settings)
		if decision.Method != playback.PlayRemux || decision.TranscodeAudio {
			t.Errorf("%s: got (%q, transcodeAudio=%v), want remux with audio copied",
				codec, decision.Method, decision.TranscodeAudio)
		}
	}
}

// Direct play hands over the original file untouched, so the flat codec claim is
// the right question there and this change must not disturb it.
func TestResolveDirectPlayUnaffectedByRemuxAudioGate(t *testing.T) {
	settings := playback.AdminSettings{TranscodeEnabled: true}
	caps := playback.ClientCapabilities{
		CodecsVideo:   []string{"hevc"},
		CodecsAudio:   []string{"aac", "ac3"},
		Containers:    []string{"mkv"},
		MaxResolution: "2160p",
	}
	file := &models.MediaFile{
		CodecVideo: "hevc", CodecAudio: "ac3", Container: "mkv", Resolution: "2160p",
	}

	if decision := playback.Resolve(file, caps, settings); decision.Method != playback.PlayDirect {
		t.Errorf("Method = %q, want direct (container matches; nothing is rewritten)", decision.Method)
	}
}

// An aliased source label against a canonical client claim must still take the
// remux route with audio copied, rather than being routed to an AAC re-encode
// before the container-safety decision is ever consulted.
func TestResolveRemuxHonorsAliasedPassthroughClaim(t *testing.T) {
	settings := playback.AdminSettings{TranscodeEnabled: true}
	caps := playback.ClientCapabilities{
		CodecsVideo:            []string{"hevc"},
		CodecsAudio:            []string{"aac"},
		AudioPassthroughCodecs: []string{"eac3"},
		Containers:             []string{"mp4"},
		MaxResolution:          "2160p",
	}
	file := &models.MediaFile{
		CodecVideo: "hevc", CodecAudio: "E-AC-3", Container: "mkv", Resolution: "2160p",
	}

	decision := playback.Resolve(file, caps, settings)
	if decision.Method != playback.PlayRemux || decision.TranscodeAudio {
		t.Fatalf("got (%q, transcodeAudio=%v), want remux with audio copied",
			decision.Method, decision.TranscodeAudio)
	}
}

// A fragile codec without passthrough evidence still converts, so alias
// tolerance widens matching without weakening the container-safety gate. DTS
// stands in for the fragile set now that the AC-3 family copies on a plain claim.
func TestResolveRemuxAliasedSourceStillConvertsWithoutEvidence(t *testing.T) {
	settings := playback.AdminSettings{TranscodeEnabled: true}
	caps := playback.ClientCapabilities{
		CodecsVideo:   []string{"hevc"},
		CodecsAudio:   []string{"aac", "dts"},
		Containers:    []string{"mp4"},
		MaxResolution: "2160p",
	}
	file := &models.MediaFile{
		CodecVideo: "hevc", CodecAudio: "DTS-HD MA", Container: "mkv", Resolution: "2160p",
	}

	decision := playback.Resolve(file, caps, settings)
	if decision.Method != playback.PlayRemux || !decision.TranscodeAudio {
		t.Fatalf("got (%q, transcodeAudio=%v), want remux with audio transcoded",
			decision.Method, decision.TranscodeAudio)
	}
}
