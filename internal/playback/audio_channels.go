package playback

// Channel layouts the AAC encoder is driven at. Anything between these snaps
// down to the next one: a client asking for 5 channels gets stereo rather than
// an odd layout no decoder is required to handle.
const (
	audioChannelsMono   = 1
	audioChannelsStereo = 2
	audioChannels51     = 6
	audioChannels71     = 8
)

// audioChannelLadder is descending, and every rung must be a layout AAC
// encoders and consumer decoders both handle.
var audioChannelLadder = []int{audioChannels71, audioChannels51, audioChannelsStereo, audioChannelsMono}

// MaxAudioChannelsSupported is the ceiling accepted from a client. Values above
// it are clamped rather than rejected: a client overstating its sink should get
// the best layout we actually produce, not a failed session.
const MaxAudioChannelsSupported = audioChannels71

// EffectiveAudioChannels resolves the channel count to encode AAC at.
//
// sourceChannels is what the input track carries, maxChannels is the client's
// declared ceiling (max_audio_channels; 0 means it did not say), and sourceCodec
// selects the fragile-lossless safety below.
//
// Three rules, in order:
//
//   - A fragile lossless source (TrueHD/MLP) is always downmixed to stereo.
//     Multichannel TrueHD decode is a known stall source, and that risk does not
//     change because a client says it can take 5.1 on the far side.
//   - Never exceed what the source has. Upmixing invents channels, costs bitrate
//     and gains nothing -- a mono source therefore stays mono rather than being
//     duplicated into stereo, which is what "never exceed" has to mean if it is
//     to mean anything.
//   - Never exceed what the client declared. A TV whose panel tops out at 5.1
//     reports 6, and handing it eight-channel AAC is exactly the "unsupported
//     audio codec" class of failure this exists to prevent.
//
// The result is always a ladder rung, so an odd ceiling snaps down instead of
// producing a layout nothing is obliged to decode. A client that declares
// nothing keeps the historical stereo default: silence about a capability is not
// evidence of it.
func EffectiveAudioChannels(sourceChannels, maxChannels int, sourceCodec string) int {
	if IsFragileLosslessAudioCodec(sourceCodec) {
		return audioChannelsStereo
	}
	if maxChannels <= 0 {
		return audioChannelsStereo
	}
	if maxChannels > MaxAudioChannelsSupported {
		maxChannels = MaxAudioChannelsSupported
	}

	ceiling := maxChannels
	if sourceChannels > 0 && sourceChannels < ceiling {
		ceiling = sourceChannels
	}
	for _, rung := range audioChannelLadder {
		if rung <= ceiling {
			return rung
		}
	}
	// Unreachable while mono is the lowest rung, but a ladder change should not
	// silently start returning 0 channels.
	return audioChannelsMono
}

// AudioBitrateForChannels is the AAC bitrate to pair with a channel count.
func AudioBitrateForChannels(channels int) string {
	switch {
	case channels >= audioChannels71:
		return "512k"
	case channels >= audioChannels51:
		return "384k"
	default:
		return "192k"
	}
}

// ac3MaxChannels is the widest layout FFmpeg's AC-3 encoder accepts. Verified
// against the encoder in our own image rather than taken from documentation:
// `ffmpeg -h encoder=ac3` lists 5.1 as the largest supported channel layout,
// with no 6.1 or 7.1 mode (ATSC A/52 has none).
const ac3MaxChannels = audioChannels51

// EffectiveAC3Channels resolves the channel count to encode AC-3 at, or 0 to
// leave the layout to FFmpeg.
//
// This deliberately does not reuse EffectiveAudioChannels. That function's
// fragile-lossless rule collapses TrueHD to stereo, which is right for AAC --
// the stall it guards against is in the TrueHD-to-AAC path -- and wrong here:
// an AC-3 target exists precisely to carry surround to an HDMI receiver, so
// forcing stereo would defeat the only reason to choose it.
//
// Three caps, tightest wins:
//
//   - Never exceed the source. Upmixing a stereo pair into a 5.1 bitstream
//     invents channels and spends bitrate saying nothing.
//   - Never exceed the client's declared ceiling (max_audio_channels). A device
//     that reported 2 must not be handed 5.1.
//   - Never exceed 5.1, which is all the encoder can do.
//
// Returns 0 when the source channel count is unknown and no client ceiling binds
// tighter than the encoder's own. FFmpeg's filter-graph negotiation then picks
// the widest layout the encoder supports that the input can fill -- which is the
// right answer, and better than the alternative of guessing 5.1 and upmixing a
// source that turns out to be stereo. Where we do know enough to choose, we
// choose: relying on negotiation is relying on it staying put.
//
// Unlike the AAC path, a client that declared nothing is left unconstrained
// rather than defaulted to stereo. Reaching this branch at all means something
// asked for an AC-3/E-AC-3 target, and that request is itself the surround
// signal the AAC default lacks.
func EffectiveAC3Channels(sourceChannels, maxChannels int) int {
	ceiling := ac3MaxChannels
	if maxChannels > 0 && maxChannels < ceiling {
		ceiling = maxChannels
	}
	if sourceChannels > 0 && sourceChannels < ceiling {
		ceiling = sourceChannels
	}
	if sourceChannels <= 0 && ceiling >= ac3MaxChannels {
		return 0
	}
	for _, rung := range audioChannelLadder {
		if rung <= ceiling {
			return rung
		}
	}
	// Unreachable while mono is the lowest rung, but a ladder change should not
	// silently start returning 0 and re-enable negotiation by accident.
	return audioChannelsMono
}

// AC3BitrateForChannels is the AC-3 bitrate to pair with a channel count.
//
// 448k is the conventional 5.1 rate (the format tops out at 640k); spending it
// on a stereo pair would only pad the stream.
func AC3BitrateForChannels(channels int) string {
	switch {
	case channels <= 0, channels >= audioChannels51:
		// 0 means the layout is left to FFmpeg, which cannot exceed 5.1 here, so
		// the surround rate is the safe pairing.
		return "448k"
	case channels >= audioChannelsStereo:
		return "192k"
	default:
		return "96k"
	}
}
