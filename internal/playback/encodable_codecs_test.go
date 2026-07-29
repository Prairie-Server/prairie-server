package playback

import "testing"

func TestSelectTargetVideoCodec(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		client    []string
		encodable []string
		want      string
	}{
		{
			name:      "hevc over h264 when both available",
			client:    []string{"h264", "hevc", "av1"},
			encodable: []string{"h264", "hevc"},
			want:      "hevc",
		},
		{
			name:      "browser without hevc stays on h264",
			client:    []string{"h264"},
			encodable: []string{"h264", "hevc"},
			want:      "h264",
		},
		{
			name:      "av1 only when both sides support it",
			client:    []string{"av1", "hevc", "h264"},
			encodable: []string{"h264", "hevc", "av1"},
			want:      "av1",
		},
		{
			name:      "av1 client without nvenc av1 falls to hevc",
			client:    []string{"av1", "hevc"},
			encodable: []string{"h264", "hevc"},
			want:      "hevc",
		},
		{
			name:      "empty client uses best encodable",
			client:    nil,
			encodable: []string{"h264", "hevc"},
			want:      "hevc",
		},
		{
			name:      "empty intersection falls back to h264",
			client:    []string{"av1"},
			encodable: []string{"h264"},
			want:      "h264",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SelectTargetVideoCodec(tc.client, tc.encodable)
			if got != tc.want {
				t.Fatalf("SelectTargetVideoCodec(%v, %v) = %q, want %q", tc.client, tc.encodable, got, tc.want)
			}
		})
	}
}

func TestNormalizeTranscodeVideoCodec(t *testing.T) {
	t.Parallel()
	if got := NormalizeTranscodeVideoCodec("copy", []string{"hevc"}, []string{"hevc", "h264"}); got != "copy" {
		t.Fatalf("copy preserved = %q", got)
	}
	if got := NormalizeTranscodeVideoCodec("", []string{"h264", "hevc"}, []string{"h264", "hevc"}); got != "hevc" {
		t.Fatalf("empty selects hevc = %q", got)
	}
	if got := NormalizeTranscodeVideoCodec("h264", []string{"hevc"}, []string{"hevc"}); got != "h264" {
		t.Fatalf("explicit request preserved = %q", got)
	}
}

func TestDetectEncodableVideoCodecs_softwareIsH264Only(t *testing.T) {
	t.Parallel()
	got := DetectEncodableVideoCodecs("ffmpeg", "none")
	if len(got) != 1 || got[0] != "h264" {
		t.Fatalf("software encodable = %v, want [h264]", got)
	}
}

func TestDetectEncodableVideoCodecs_av1RequiresWorkingNVENCEncode(t *testing.T) {
	setupHWAccelTest(t)

	listedOnly := writeFakeFFmpeg(t, fakeFFmpegProbe{
		cuda: true, h264NVENC: true, hevcNVENC: true, av1NVENC: true,
		scaleCUDA: true, uploadCUDA: true, smokeOK: true, av1SmokeOK: false,
	})
	got := DetectEncodableVideoCodecs(listedOnly.path, "nvenc")
	if containsString(got, "av1") {
		t.Fatalf("Ampere-style listed-but-broken av1_nvenc must not advertise av1: %v", got)
	}
	if !containsString(got, "h264") || !containsString(got, "hevc") {
		t.Fatalf("h264/hevc still required: %v", got)
	}

	resetNVENCProbeCacheForTest()
	working := writeFakeFFmpeg(t, fakeFFmpegProbe{
		cuda: true, h264NVENC: true, hevcNVENC: true, av1NVENC: true,
		scaleCUDA: true, uploadCUDA: true, smokeOK: true, av1SmokeOK: true,
	})
	got = DetectEncodableVideoCodecs(working.path, "nvenc")
	if !containsString(got, "av1") {
		t.Fatalf("working av1_nvenc smoke should advertise av1: %v", got)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
