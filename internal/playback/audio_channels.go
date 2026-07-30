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
