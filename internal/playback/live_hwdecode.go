package playback

import (
	"strings"
	"sync"
)

// Hardware decode modes for live sessions.
const (
	// HWDecodeAuto uses a hardware decoder when the ffmpeg build has one.
	HWDecodeAuto = "auto"
	// HWDecodeOn assumes the decoder exists and skips the probe.
	HWDecodeOn = "on"
	// HWDecodeOff forces software decode.
	HWDecodeOff = "off"
)

// NVENC decoding a live broadcast is the difference between keeping up and
// falling behind: software MPEG-2 decode at 59.94fps costs more CPU than the
// h264 encode that follows it, and a live transcode that runs below realtime
// starves the player no matter how fast the encoder is.
//
// -hwaccel cuda alone lets ffmpeg choose, and for MPEG-2 it commonly lands on
// the software decoder, so the frames make a CPU round trip before they reach
// the GPU. Naming the CUVID decoder pins the whole pipeline on the GPU:
// NVDEC decode → scale_cuda → h264_nvenc, with no download in between.
var cuvidDecoders = map[string]string{
	"mpeg2video": "mpeg2_cuvid",
	"h264":       "h264_cuvid",
	"hevc":       "hevc_cuvid",
	"vc1":        "vc1_cuvid",
	"mpeg4":      "mpeg4_cuvid",
	"vp9":        "vp9_cuvid",
	"av1":        "av1_cuvid",
}

// normalizeVideoCodecName maps the codec spellings tuners and probes report
// onto ffmpeg's canonical decoder names.
func normalizeVideoCodecName(codec string) string {
	normalized := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").
		Replace(strings.ToLower(strings.TrimSpace(codec)))
	switch normalized {
	case "mpeg2video", "mpeg2", "mp2v":
		return "mpeg2video"
	case "h264", "avc", "avc1", "x264":
		return "h264"
	case "hevc", "h265", "x265", "hev1", "hvc1":
		return "hevc"
	case "vc1", "wvc1":
		return "vc1"
	case "mpeg4", "divx", "xvid":
		return "mpeg4"
	case "vp9":
		return "vp9"
	case "av1", "av01":
		return "av1"
	default:
		return normalized
	}
}

// hardwareVideoDecoder returns the decoder that keeps a source codec on the
// GPU for the given acceleration method, or "" when there isn't one.
//
// Only NVENC gets an explicit decoder. The QSV and VAAPI argument builders
// already request `-hwaccel vaapi -hwaccel_output_format vaapi`, which engages
// their hardware decoders for the whole pipeline; naming a decoder on top of
// that conflicts with the device setup those paths perform.
func hardwareVideoDecoder(hwAccel, sourceCodec string) string {
	if hwAccel != hwAccelNVENC {
		return ""
	}
	return cuvidDecoders[normalizeVideoCodecName(sourceCodec)]
}

var decoderProbeCache = struct {
	sync.Mutex
	byKey map[string]bool
}{byKey: map[string]bool{}}

// ffmpegHasDecoder reports whether the ffmpeg build ships a decoder. Results
// are cached per binary because `-decoders` is a process spawn.
func ffmpegHasDecoder(ffmpegPath, decoder string) bool {
	if decoder == "" {
		return false
	}
	ffmpegPath = normalizeFFmpegPath(ffmpegPath)
	key := ffmpegPath + "\x00" + decoder

	decoderProbeCache.Lock()
	cached, ok := decoderProbeCache.byKey[key]
	decoderProbeCache.Unlock()
	if ok {
		return cached
	}

	output, err := runFFmpegProbe(ffmpegPath, "-hide_banner", "-decoders")
	available := err == nil && ffmpegOutputHasToken(output, decoder)

	decoderProbeCache.Lock()
	decoderProbeCache.byKey[key] = available
	decoderProbeCache.Unlock()
	return available
}

func resetDecoderProbeCacheForTest() {
	decoderProbeCache.Lock()
	decoderProbeCache.byKey = map[string]bool{}
	decoderProbeCache.Unlock()
}

// resolveLiveVideoDecoder picks the hardware decoder for a live encode.
// Returns "" for software decode.
func resolveLiveVideoDecoder(mode, hwAccel, sourceCodec, ffmpegPath string) string {
	if strings.EqualFold(strings.TrimSpace(mode), HWDecodeOff) {
		return ""
	}
	decoder := hardwareVideoDecoder(hwAccel, sourceCodec)
	if decoder == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(mode), HWDecodeOn) {
		return decoder
	}
	if !ffmpegHasDecoder(ffmpegPath, decoder) {
		return ""
	}
	return decoder
}
