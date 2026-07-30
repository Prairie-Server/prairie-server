package playback

import "strings"

// Remux output containers. These are the only formats the progressive remux
// will write, because each has to satisfy two constraints at once: FFmpeg must
// be able to mux it to a non-seekable pipe, and it must carry the codecs a
// remux copies without re-encoding.
const (
	RemuxContainerMP4 = "mp4"
	RemuxContainerMKV = "mkv"
)

// DefaultRemuxContainer is the fallback when the client's declared containers
// give us nothing better to go on. MP4 is the safest default because every
// client that can play video at all can play it.
const DefaultRemuxContainer = RemuxContainerMP4

// normalizeRemuxContainer maps the spellings clients and probes use onto our
// two canonical names, returning "" for anything we will not write.
//
// webm is deliberately absent. It is a Matroska subset restricted to VP8/VP9/AV1
// plus Vorbis/Opus, so it cannot carry the AC-3 or H.264 a remux copies, and
// choosing it would turn a stream copy into a re-encode.
func normalizeRemuxContainer(container string) string {
	switch strings.ToLower(strings.TrimSpace(container)) {
	case "mp4", "m4v", "mov", "isom":
		return RemuxContainerMP4
	case "mkv", "matroska", "x-matroska":
		return RemuxContainerMKV
	default:
		return ""
	}
}

// RemuxFFmpegFormat is the `-f` argument for a canonical container name.
// Matroska's muxer is spelled "matroska", not "mkv". An unrecognized or empty
// name yields the MP4 default, so a session predating the container choice --
// or reconstructed from an older token -- behaves exactly as it did before.
func RemuxFFmpegFormat(container string) string {
	if normalizeRemuxContainer(container) == RemuxContainerMKV {
		return "matroska"
	}
	return "mp4"
}

// RemuxOutputContainer chooses the container the progressive remux writes.
//
// It prefers to keep the source container when the client declared support for
// it, and falls back to MP4 otherwise. That single rule covers both reasons a
// session remuxes at all:
//
//   - The client cannot play the source container. It is therefore not in the
//     declared list, so this returns MP4 -- which is what such a session needed
//     in the first place.
//   - The client can play the container but we still have to rewrite, typically
//     to map a specific audio track that direct play cannot select. Here the
//     source container is declared, so it is kept.
//
// Keeping the source container in that second case matters beyond tidiness.
// Rewriting into MP4 is not free: MP4 must be fragmented to stream over a pipe
// (it has no seekable moov to patch), and several codecs a remux copies are
// spec-legal in MP4 but unreliable in practice -- AC-3, E-AC-3, DTS and TrueHD
// all sit in that category, which is why remuxAudioCopySafe treats a bare codec
// claim as insufficient evidence for them. Matroska has neither problem: it
// streams unfragmented and carries those codecs unambiguously. So when the
// client has already told us it can demux the source container, changing
// containers only adds failure modes.
//
// Trusting the declared list adds no new trust: Resolve already serves that
// exact container byte-for-byte via direct play when the client claims it (see
// containerOK in Case 1). A client that over-claims a container is already
// broken for direct play, and this does not make it more so.
func RemuxOutputContainer(sourceContainer string, declaredContainers []string) string {
	source := normalizeRemuxContainer(sourceContainer)
	if source == "" {
		return DefaultRemuxContainer
	}
	for _, declared := range declaredContainers {
		if normalizeRemuxContainer(declared) == source {
			return source
		}
	}
	return DefaultRemuxContainer
}
