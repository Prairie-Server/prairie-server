package playback

import (
	"strconv"
	"strings"
)

// Bitrate advertised when a session has no explicit cap. BANDWIDTH is required
// on EXT-X-STREAM-INF, and a player that trusts it for its initial buffer
// arithmetic does better with a plausible 4K-ish figure than with zero.
const defaultAdvertisedBandwidthBps = 8_000_000

// BuildMasterManifest wraps a media playlist in a single-variant master
// playlist.
//
// Samsung's PlusPlayer decides what decoder pipeline to build during prepare,
// before it fetches any media. Handed a bare media playlist it has no CODECS or
// RESOLUTION to work from, and prepare fails — silently, because the plugin
// drops a failed prepare without emitting an error (see PlusPlayer::
// OnPrepareDone, which only calls SendInitialized when ret is true). The client
// then hangs in initialize() until its own timeout. Every browser and hls.js
// path tolerates a bare media playlist, which is why this only ever showed up
// on the TV.
//
// mediaURI is written verbatim, so callers keep control of the query string the
// player must carry (the signed stream token rides there).
func BuildMasterManifest(mediaURI string, opts TranscodeOpts) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	// 3 is all a single-variant master needs; the media playlist carries its own
	// higher version for fMP4.
	b.WriteString("#EXT-X-VERSION:3\n")
	b.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")

	b.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.Itoa(advertisedBandwidthBps(opts)))
	if res := resolutionAttribute(opts.TargetResolution); res != "" {
		b.WriteString(",RESOLUTION=" + res)
	}
	if codecs := streamInfCodecs(opts); codecs != "" {
		b.WriteString(`,CODECS="` + codecs + `"`)
	}
	b.WriteString("\n" + mediaURI + "\n")
	return []byte(b.String())
}

func advertisedBandwidthBps(opts TranscodeOpts) int {
	if opts.TargetBitrateKbps > 0 {
		return opts.TargetBitrateKbps * 1000
	}
	return defaultAdvertisedBandwidthBps
}

// resolutionAttribute maps a ladder token to the WIDTHxHEIGHT form
// EXT-X-STREAM-INF wants. Unknown or empty tokens are omitted rather than
// guessed: RESOLUTION is optional, and a wrong one is worse than none.
func resolutionAttribute(target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "2160p":
		return "3840x2160"
	case "1440p":
		return "2560x1440"
	case "1080p":
		return "1920x1080"
	case "720p":
		return "1280x720"
	case "480p":
		return "854x480"
	case "360p":
		return "640x360"
	default:
		return ""
	}
}

// streamInfCodecs builds the CODECS attribute.
//
// For a copy session the bytes on the wire are the source codec, so that is
// what gets advertised. Values are the generic RFC 6381 forms: a player needs
// the family to pick a decoder, and inventing precise profile/level fields from
// data we have not parsed would risk advertising something the stream does not
// match. An unrecognised codec contributes nothing rather than a guess, and if
// nothing is recognised the attribute is omitted entirely.
func streamInfCodecs(opts TranscodeOpts) string {
	video := opts.TargetCodecVideo
	if strings.EqualFold(video, "copy") {
		video = opts.SourceVideoCodec
	}

	parts := make([]string, 0, 2)
	if tag := videoCodecTag(video); tag != "" {
		parts = append(parts, tag)
	}
	if tag := audioCodecTag(opts.TargetCodecAudio); tag != "" {
		parts = append(parts, tag)
	}
	return strings.Join(parts, ",")
}

func videoCodecTag(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "avc", "avc1":
		return "avc1.4d401f"
	case "hevc", "h265", "hvc1":
		return "hvc1.1.6.L123.b0"
	case "av1", "av01":
		return "av01.0.08M.08"
	case "vp9":
		return "vp09.00.10.08"
	default:
		return ""
	}
}

func audioCodecTag(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "aac", "mp4a":
		return "mp4a.40.2"
	case "ac3":
		return "ac-3"
	case "eac3":
		return "ec-3"
	case "opus":
		return "opus"
	case "flac":
		return "fLaC"
	default:
		return ""
	}
}

// MasterManifestMediaURI is the media-playlist path a master playlist points at,
// preserving rawQuery so the player carries the stream token forward.
func MasterManifestMediaURI(rawQuery string) string {
	if rawQuery == "" {
		return "media.m3u8"
	}
	return "media.m3u8?" + rawQuery
}
