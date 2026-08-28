package livetv

import (
	"strings"
	"unicode"

	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

// resolveGuideStationID picks a Schedules Direct station for an HDHomeRun-style
// channel (number like "2.1", callsign like "KDTN-DT") using the lineup map.
// Existing GuideStationID values are treated as user overrides.
func resolveGuideStationID(ch Channel, detail *schedulesdirect.LineupDetail) string {
	if id := strings.TrimSpace(ch.GuideStationID); id != "" {
		return id
	}
	if detail == nil {
		return ""
	}

	displayNum := normalizeChannelNumber(channelDisplayNumber(ch))
	stationByNumber := map[string]string{}
	entriesByMajor := map[string][]schedulesdirect.LineupMapEntry{}
	for _, m := range detail.Map {
		num := normalizeChannelNumber(m.Channel)
		if num == "" || strings.TrimSpace(m.StationID) == "" {
			continue
		}
		stationByNumber[num] = m.StationID
		major := channelMajor(num)
		entriesByMajor[major] = append(entriesByMajor[major], m)
	}
	if id := stationByNumber[displayNum]; id != "" {
		return id
	}

	wantCall := normalizeCallsign(ch.Callsign)
	if wantCall == "" {
		wantCall = normalizeCallsign(ch.Name)
	}
	if wantCall != "" {
		if id := uniqueCallsignMatch(wantCall, detail.Stations, true); id != "" {
			return id
		}
		// Soft prefix match only when unique (e.g. HDHR "KDTN" vs SD "KDTNDT"
		// already covered by normalize; this helps odd Name fields).
		if id := uniqueCallsignMatch(wantCall, detail.Stations, false); id != "" {
			return id
		}
		// Disambiguate multiple exact roots with channel major.
		exact := callsignMatches(wantCall, detail.Stations, true)
		if len(exact) > 1 {
			major := channelMajor(displayNum)
			filtered := make([]string, 0)
			for _, id := range exact {
				for _, m := range entriesByMajor[major] {
					if m.StationID == id {
						filtered = append(filtered, id)
						break
					}
				}
			}
			if len(filtered) == 1 {
				return filtered[0]
			}
			pool := exact
			if len(filtered) > 0 {
				pool = filtered
			}
			if id := preferPrimarySubchannel(pool, detail.Map); id != "" {
				return id
			}
		}
	}

	// SD OTA sometimes lists major-only "5" for HDHR "5.1".
	major := channelMajor(displayNum)
	if entries := entriesByMajor[major]; len(entries) == 1 {
		return entries[0].StationID
	}
	if id := preferPrimarySubchannel(nil, entriesByMajor[major]); id != "" {
		return id
	}
	return ""
}

func uniqueCallsignMatch(want string, stations []schedulesdirect.Station, exactOnly bool) string {
	matches := callsignMatches(want, stations, exactOnly)
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func callsignMatches(want string, stations []schedulesdirect.Station, exactOnly bool) []string {
	out := make([]string, 0)
	seen := map[string]bool{}
	for _, st := range stations {
		if !stationCallsignMatches(want, st, exactOnly) {
			continue
		}
		if seen[st.StationID] {
			continue
		}
		seen[st.StationID] = true
		out = append(out, st.StationID)
	}
	return out
}

func stationCallsignMatches(want string, st schedulesdirect.Station, exactOnly bool) bool {
	if want == "" {
		return false
	}
	for _, candidate := range []string{st.Callsign, st.Name} {
		got := normalizeCallsign(candidate)
		if got == "" {
			continue
		}
		if got == want {
			return true
		}
		if exactOnly {
			continue
		}
		if min(len(got), len(want)) < 4 {
			continue
		}
		if strings.HasPrefix(got, want) || strings.HasPrefix(want, got) {
			return true
		}
	}
	return false
}

func preferPrimarySubchannel(stationIDs []string, entries []schedulesdirect.LineupMapEntry) string {
	allowed := map[string]bool{}
	for _, id := range stationIDs {
		allowed[id] = true
	}
	restrict := len(allowed) > 0
	for _, m := range entries {
		if restrict && !allowed[m.StationID] {
			continue
		}
		num := normalizeChannelNumber(m.Channel)
		if strings.HasSuffix(num, ".1") {
			return m.StationID
		}
	}
	return ""
}

func channelMajor(num string) string {
	n := normalizeChannelNumber(num)
	if i := strings.Index(n, "."); i >= 0 {
		if i == 0 {
			return "0"
		}
		return n[:i]
	}
	return n
}

// normalizeCallsign folds HDHomeRun GuideName values like "KDTN-DT" and
// Schedules Direct callsigns like "KDTNDT" into a comparable root.
func normalizeCallsign(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	s := b.String()
	for _, suf := range []string{"DT", "HD", "TV", "SD", "CD"} {
		if strings.HasSuffix(s, suf) && len(s)-len(suf) >= 3 {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
