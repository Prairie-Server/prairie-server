package jellycompat

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/models"
)

type codecProfileCompatibility struct {
	VideoSupported bool
	AudioSupported bool
}

func (c codecProfileCompatibility) supportsDirectPlay() bool {
	return c.VideoSupported && c.AudioSupported
}

type conditionValue struct {
	text   string
	number int
	hasNum bool
}

type conditionValues map[string]conditionValue

func (c *ProfileCondition) UnmarshalJSON(data []byte) error {
	var raw struct {
		Condition  string `json:"Condition"`
		Property   string `json:"Property"`
		Value      any    `json:"Value"`
		IsRequired bool   `json:"IsRequired"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value, err := stringifyConditionValue(raw.Value)
	if err != nil {
		return err
	}
	c.Condition = raw.Condition
	c.Property = raw.Property
	c.Value = value
	c.IsRequired = raw.IsRequired
	return nil
}

func stringifyConditionValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("marshal profile condition value: %w", err)
		}
		return string(data), nil
	}
}

func (p DeviceProfile) codecProfileCompatibility(version catalog.FileVersion, audioStreamIndex *int) codecProfileCompatibility {
	compat := codecProfileCompatibility{VideoSupported: true, AudioSupported: true}
	if len(p.CodecProfiles) == 0 {
		return compat
	}

	values := buildConditionValues(version, audioStreamIndex)
	for _, profile := range p.CodecProfiles {
		target := codecProfileTarget(profile)
		if target == "" || !codecProfileApplies(profile, version, audioStreamIndex) {
			continue
		}
		if !conditionsMatch(profile.ApplyConditions, values) {
			continue
		}
		if conditionsMatch(profile.Conditions, values) {
			continue
		}

		if target == "audio" || conditionsOnlyTargetAudio(profile.Conditions) {
			compat.AudioSupported = false
		} else {
			compat.VideoSupported = false
		}
	}
	return compat
}

func codecProfileTarget(profile CodecProfile) string {
	typ := normalizeConditionToken(profile.Type)
	switch typ {
	case "videoaudio", "audio":
		return "audio"
	case "", "video":
		return "video"
	default:
		return ""
	}
}

func codecProfileApplies(profile CodecProfile, version catalog.FileVersion, audioStreamIndex *int) bool {
	if strings.TrimSpace(profile.Codec) == "" {
		return true
	}
	switch codecProfileTarget(profile) {
	case "audio":
		return matchesCSV(profile.Codec, compatAudioCodec(version, audioStreamIndex))
	case "video":
		return matchesCSV(profile.Codec, version.CodecVideo)
	default:
		return false
	}
}

func conditionsOnlyTargetAudio(conditions []ProfileCondition) bool {
	if len(conditions) == 0 {
		return false
	}
	for _, condition := range conditions {
		if normalizeConditionToken(condition.Property) != "audiochannels" {
			return false
		}
	}
	return true
}

func conditionsMatch(conditions []ProfileCondition, values conditionValues) bool {
	for _, condition := range conditions {
		if !conditionMatches(condition, values) {
			return false
		}
	}
	return true
}

func conditionMatches(condition ProfileCondition, values conditionValues) bool {
	actual, ok := values[normalizeConditionToken(condition.Property)]
	if !ok {
		return false
	}

	switch normalizeConditionToken(condition.Condition) {
	case "equals":
		return stringInConditionSet(actual.text, condition.Value)
	case "equalsany", "incollection":
		return stringInConditionSet(actual.text, condition.Value)
	case "notequals":
		return !stringInConditionSet(actual.text, condition.Value)
	case "notequalsany", "notincollection":
		return !stringInConditionSet(actual.text, condition.Value)
	case "lessthanequal", "lessthanorequal", "lowerthanequal", "lowerthanorequal":
		want, ok := firstConditionNumber(condition.Value)
		return ok && actual.hasNum && actual.number <= want
	case "greaterthanequal", "greaterthanorequal":
		want, ok := firstConditionNumber(condition.Value)
		return ok && actual.hasNum && actual.number >= want
	case "lessthan", "lowerthan":
		want, ok := firstConditionNumber(condition.Value)
		return ok && actual.hasNum && actual.number < want
	case "greaterthan":
		want, ok := firstConditionNumber(condition.Value)
		return ok && actual.hasNum && actual.number > want
	default:
		return false
	}
}

func buildConditionValues(version catalog.FileVersion, audioStreamIndex *int) conditionValues {
	video := compatPrimaryVideoTrack(version)
	audio := compatAudioTrack(version, audioStreamIndex)

	values := conditionValues{
		"videorangetype": {text: compatVideoRangeType(video, version.HDR)},
		"videoprofile":   {text: video.Profile},
		"videolevel":     intConditionValue(video.Level),
		"refframes":      intConditionValue(video.ReferenceFrames),
		"width":          intConditionValue(video.Width),
		"height":         intConditionValue(video.Height),
		"videobitdepth":  intConditionValue(video.BitDepth),
		"audiochannels":  intConditionValue(audio.Channels),
	}
	return values
}

func intConditionValue(value int) conditionValue {
	return conditionValue{text: strconv.Itoa(value), number: value, hasNum: value > 0}
}

func compatPrimaryVideoTrack(version catalog.FileVersion) models.VideoTrack {
	if len(version.VideoTracks) > 0 {
		return version.VideoTracks[0]
	}
	return models.VideoTrack{Codec: version.CodecVideo}
}

func compatAudioTrack(version catalog.FileVersion, streamIndex *int) models.AudioTrack {
	if streamIndex != nil {
		audioIndex := *streamIndex - len(version.VideoTracks)
		if audioIndex >= 0 && audioIndex < len(version.AudioTracks) {
			return version.AudioTracks[audioIndex]
		}
	}
	for _, track := range version.AudioTracks {
		if track.Default {
			return track
		}
	}
	if len(version.AudioTracks) > 0 {
		return version.AudioTracks[0]
	}
	return models.AudioTrack{Codec: version.CodecAudio}
}

func compatAudioCodec(version catalog.FileVersion, streamIndex *int) string {
	if track := compatAudioTrack(version, streamIndex); track.Codec != "" {
		return track.Codec
	}
	return version.CodecAudio
}

func compatVideoRange(track models.VideoTrack, versionHDR bool) string {
	rangeType := compatVideoRangeType(track, versionHDR)
	switch rangeType {
	case "SDR":
		return "SDR"
	case "Unknown":
		return "Unknown"
	default:
		return "HDR"
	}
}

func compatVideoRangeType(track models.VideoTrack, versionHDR bool) string {
	if value := strings.TrimSpace(track.VideoRangeType); value != "" {
		return value
	}
	if profile := compatDolbyVisionProfile(track); profile > 0 {
		switch profile {
		case 5:
			return "DOVI"
		case 7:
			if track.HDR10Plus {
				return "DOVIWithELHDR10Plus"
			}
			return "DOVIWithEL"
		case 8:
			if track.HDR10Plus {
				return "DOVIWithHDR10Plus"
			}
			switch track.DVBLCompatID {
			case 1:
				return "DOVIWithHDR10"
			case 2:
				return "DOVIWithSDR"
			case 4:
				return "DOVIWithHLG"
			default:
				if compatIsHLG(track.ColorTransfer) {
					return "DOVIWithHLG"
				}
				if compatIsHDR(track.ColorTransfer) || versionHDR || strings.EqualFold(track.VideoRange, "HDR") {
					return "DOVIWithHDR10"
				}
				return "DOVIWithSDR"
			}
		default:
			return "DOVI"
		}
	}
	if track.HDR10Plus {
		return "HDR10Plus"
	}
	if compatIsHLG(track.ColorTransfer) {
		return "HLG"
	}
	if compatIsHDR(track.ColorTransfer) || versionHDR || strings.EqualFold(track.VideoRange, "HDR") {
		return "HDR10"
	}
	return "SDR"
}

func compatDolbyVisionProfile(track models.VideoTrack) int {
	if track.DVProfile > 0 {
		return track.DVProfile
	}
	raw := strings.ToLower(track.DolbyVision)
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if value, err := strconv.Atoi(field); err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func compatIsHDR(colorTransfer string) bool {
	value := strings.ToLower(colorTransfer)
	return strings.Contains(value, "smpte2084") || strings.Contains(value, "arib-std-b67")
}

func compatIsHLG(colorTransfer string) bool {
	return strings.Contains(strings.ToLower(colorTransfer), "arib-std-b67")
}

func stringInConditionSet(actual, rawSet string) bool {
	actual = normalizeConditionToken(actual)
	for _, value := range splitConditionSet(rawSet) {
		if actual == normalizeConditionToken(value) {
			return true
		}
	}
	return false
}

func splitConditionSet(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == '|' || r == ','
	})
}

func firstConditionNumber(raw string) (int, bool) {
	for _, value := range splitConditionSet(raw) {
		number, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return number, true
		}
	}
	return 0, false
}

func normalizeConditionToken(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
