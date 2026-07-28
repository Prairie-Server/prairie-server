package playback

import (
	"strings"
)

// Preferred encode targets when the client omits target_codec_video.
// HEVC over H.264 for quality-per-bitrate; AV1 only when both sides support it
// (NVENC AV1 on Ada+, never CPU AV1 for realtime).
var preferredTranscodeVideoCodecs = []string{"av1", "hevc", "h264"}

// DetectEncodableVideoCodecs returns video codecs this host can realtime-encode
// for playback transcodes, given the resolved hw accel mode.
//
// AV1 is included only when av1_nvenc is present. Software (none) advertises
// h264 only — libx265/libaom are not suitable for interactive TV playback.
func DetectEncodableVideoCodecs(ffmpegPath, hwAccel string) []string {
	resolved := ResolveHWAccelWithFFmpeg(hwAccel, ffmpegPath)
	switch resolved {
	case hwAccelNVENC:
		out := []string{"h264", "hevc"}
		if ffmpegHasEncoderToken(ffmpegPath, "av1_nvenc") {
			out = append(out, "av1")
		}
		return out
	case "qsv", hwAccelVAAPI:
		return []string{"h264", "hevc"}
	default:
		return []string{"h264"}
	}
}

func ffmpegHasEncoderToken(ffmpegPath, encoder string) bool {
	ffmpegPath = normalizeFFmpegPath(ffmpegPath)
	output, err := runFFmpegProbe(ffmpegPath, "-hide_banner", "-encoders")
	if err != nil {
		return false
	}
	return ffmpegOutputHasToken(output, encoder)
}

// SelectTargetVideoCodec picks the best codec in clientCodecs ∩ encodableCodecs
// using preferredTranscodeVideoCodecs order. Falls back to h264 when the
// intersection is empty so older clients keep working.
func SelectTargetVideoCodec(clientCodecs, encodableCodecs []string) string {
	encodable := make(map[string]struct{}, len(encodableCodecs))
	for _, codec := range encodableCodecs {
		key := strings.ToLower(strings.TrimSpace(codec))
		if key == "" || key == "copy" {
			continue
		}
		encodable[key] = struct{}{}
	}
	client := make(map[string]struct{}, len(clientCodecs))
	for _, codec := range clientCodecs {
		key := strings.ToLower(strings.TrimSpace(codec))
		if key == "" || key == "copy" {
			continue
		}
		client[key] = struct{}{}
	}
	for _, codec := range preferredTranscodeVideoCodecs {
		_, okClient := client[codec]
		_, okServer := encodable[codec]
		if len(client) == 0 {
			// No client caps stored — still honor encoder availability.
			if okServer {
				return codec
			}
			continue
		}
		if okClient && okServer {
			return codec
		}
	}
	return "h264"
}

// NormalizeTranscodeVideoCodec resolves an empty / omitted target to the best
// capability-based encode codec. "copy" is preserved.
func NormalizeTranscodeVideoCodec(requested string, clientCodecs, encodableCodecs []string) string {
	trimmed := strings.ToLower(strings.TrimSpace(requested))
	if trimmed == "copy" {
		return "copy"
	}
	if trimmed != "" {
		return trimmed
	}
	return SelectTargetVideoCodec(clientCodecs, encodableCodecs)
}
