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
// before it fetches any media, and a bare media playlist gives it no variant to
// work from. Prepare then fails silently: the plugin's OnPrepareDone only calls
// SendInitialized when ret is true and emits nothing otherwise, so the Dart
// completer never completes and never errors and the client hangs in
// initialize() until its own timeout. Every browser and hls.js path accepts a
// bare media playlist, which is why this only ever showed up on the TV.
//
// CODECS is deliberately absent. RFC 6381 identifiers are not codec-family
// names: avc1.4d401f pins Main profile at level 3.1, av01.0.08M.08 pins profile
// 0 at level 4.0 Main tier 8-bit, and so on. The session carries only a codec
// family ("av1", "hevc"), not the profile, tier, level and bit depth those
// strings encode, so any value emitted from what we have here would be a guess
// that a strict client is entitled to believe. Advertising a mismatched decoder
// configuration is worse than advertising none, and the attribute is optional.
// Adding it properly means threading the probed stream parameters (Profile,
// Level, and bit depth on the video stream model) through TranscodeOpts and
// deriving exact values -- worth doing, but not by inventing them here.
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
// EXT-X-STREAM-INF wants.
//
// Only meaningful for an encode, which is scaling to exactly this rung. Copy
// sessions leave TargetResolution empty (the source dimensions pass through
// untouched) and so advertise no RESOLUTION, which is correct: the attribute is
// optional and the alternative would be restating a size we have not read.
// Unknown tokens are omitted for the same reason.
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

// MasterManifestMediaURI is the media-playlist path a master playlist points at,
// preserving rawQuery so the player carries the stream token forward.
func MasterManifestMediaURI(rawQuery string) string {
	if rawQuery == "" {
		return "media.m3u8"
	}
	return "media.m3u8?" + rawQuery
}
