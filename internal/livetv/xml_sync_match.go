package livetv

import (
	"strings"

	"github.com/prairie-server/prairie-server/internal/livetv/gracenote"
)

// resolveXMLSyncStationID maps an HDHomeRun-style channel onto a Gracenote
// channelId using number/callsign. Existing GuideStationID is an override.
func resolveXMLSyncStationID(ch Channel, stations []gracenote.Channel) string {
	if id := strings.TrimSpace(ch.GuideStationID); id != "" {
		return id
	}
	displayNum := normalizeChannelNumber(channelDisplayNumber(ch))
	byNumber := map[string]string{}
	byMajor := map[string][]gracenote.Channel{}
	for _, st := range stations {
		id := strings.TrimSpace(st.ChannelID)
		if id == "" {
			continue
		}
		num := normalizeChannelNumber(st.ChannelNo)
		if num != "" {
			byNumber[num] = id
			major := channelMajor(num)
			byMajor[major] = append(byMajor[major], st)
		}
	}
	if id := byNumber[displayNum]; id != "" {
		return id
	}

	wantCall := normalizeCallsign(ch.Callsign)
	if wantCall == "" {
		wantCall = normalizeCallsign(ch.Name)
	}
	if wantCall != "" {
		matches := make([]string, 0)
		seen := map[string]bool{}
		for _, st := range stations {
			got := normalizeCallsign(st.CallSign)
			if got == "" || got != wantCall || seen[st.ChannelID] {
				continue
			}
			seen[st.ChannelID] = true
			matches = append(matches, st.ChannelID)
		}
		if len(matches) == 1 {
			return matches[0]
		}
		if len(matches) > 1 {
			major := channelMajor(displayNum)
			filtered := make([]string, 0)
			for _, id := range matches {
				for _, st := range byMajor[major] {
					if st.ChannelID == id {
						filtered = append(filtered, id)
						break
					}
				}
			}
			if len(filtered) == 1 {
				return filtered[0]
			}
		}
	}

	major := channelMajor(displayNum)
	if entries := byMajor[major]; len(entries) == 1 {
		return entries[0].ChannelID
	}
	for _, st := range byMajor[major] {
		if strings.HasSuffix(normalizeChannelNumber(st.ChannelNo), ".1") {
			return st.ChannelID
		}
	}
	return ""
}
