package livetv

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

func TestNormalizeCallsign(t *testing.T) {
	cases := map[string]string{
		"KDTN-DT": "KDTN",
		"KDTNDT":  "KDTN",
		"KDFW-DT": "KDFW",
		"KDFWDT":  "KDFW",
		"KING-HD": "KING",
		"king hd": "KING",
		"WFAA":    "WFAA",
		"":        "",
	}
	for in, want := range cases {
		if got := normalizeCallsign(in); got != want {
			t.Fatalf("normalizeCallsign(%q)=%q want %q", in, got, want)
		}
	}
}

func TestResolveGuideStationIDFromHDHomeRun(t *testing.T) {
	detail := &schedulesdirect.LineupDetail{
		Map: []schedulesdirect.LineupMapEntry{
			{Channel: "2.1", StationID: "100"},
			{Channel: "2.2", StationID: "101"},
			{Channel: "4.1", StationID: "200"},
			{Channel: "5", StationID: "300"},
			{Channel: "7.1", StationID: "400"},
			{Channel: "7.2", StationID: "401"},
			{Channel: "", StationID: "skip"},
			{Channel: "9.1", StationID: ""},
		},
		Stations: []schedulesdirect.Station{
			{StationID: "100", Callsign: "KDTNDT", Name: "KDTN-DT"},
			{StationID: "101", Callsign: "KDTN2", Name: "KDTN-DT2"},
			{StationID: "200", Callsign: "KDFWDT", Name: "KDFW-DT"},
			{StationID: "300", Callsign: "KXAS", Name: "KXAS"},
			{StationID: "400", Callsign: "WFAA", Name: "WFAA-DT"},
			{StationID: "401", Callsign: "WFAA", Name: "WFAA-DT2"},
			{StationID: "500", Callsign: "LONGNAMEEXTRA", Name: ""},
		},
	}

	t.Run("exact channel number from HDHR 2.1", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "2.1", Callsign: "KDTN-DT"}, detail)
		if got != "100" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("HDHR KDTN-DT maps to SD KDTNDT by callsign", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "99.1", Callsign: "KDTN-DT"}, detail)
		if got != "100" {
			t.Fatalf("got %q want 100", got)
		}
	})

	t.Run("name fallback when callsign empty", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "99.1", Name: "KDFW-DT"}, detail)
		if got != "200" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("subchannel callsign KDTN2", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "99.2", Callsign: "KDTN2"}, detail)
		if got != "101" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("soft prefix unique match", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "99.9", Callsign: "LONGNAME"}, detail)
		if got != "500" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("duplicate callsign disambiguated by major", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "7.2", Callsign: "WFAA"}, detail)
		// exact number wins first
		if got != "401" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("duplicate callsign prefers .1 when number unknown", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "70.9", Callsign: "WFAA"}, detail)
		if got != "400" {
			t.Fatalf("got %q want primary .1", got)
		}
	})

	t.Run("major-only SD channel maps HDHR 5.1", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "5.1", Callsign: "KXAS-DT"}, detail)
		if got != "300" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("user override wins", func(t *testing.T) {
		got := resolveGuideStationID(Channel{
			Number: "2.1", Callsign: "KDTN-DT", GuideStationID: "override",
		}, detail)
		if got != "override" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("nil detail", func(t *testing.T) {
		if got := resolveGuideStationID(Channel{Number: "2.1"}, nil); got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "88.1", Callsign: "ZZZZ"}, detail)
		if got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("number override used for matching", func(t *testing.T) {
		override := "4.1"
		got := resolveGuideStationID(Channel{Number: "99", NumberOverride: &override, Callsign: "X"}, detail)
		if got != "200" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("duplicate callsign filtered to unique major", func(t *testing.T) {
		dup := &schedulesdirect.LineupDetail{
			Map: []schedulesdirect.LineupMapEntry{
				{Channel: "7.1", StationID: "400"},
				{Channel: "8.1", StationID: "401"},
			},
			Stations: []schedulesdirect.Station{
				{StationID: "400", Callsign: "WFAA"},
				{StationID: "401", Callsign: "WFAA"},
			},
		}
		got := resolveGuideStationID(Channel{Number: "7.9", Callsign: "WFAA"}, dup)
		if got != "400" {
			t.Fatalf("got %q want major-filtered 400", got)
		}
	})

	t.Run("duplicate callsign filtered pool prefers .1", func(t *testing.T) {
		dup := &schedulesdirect.LineupDetail{
			Map: []schedulesdirect.LineupMapEntry{
				{Channel: "7.1", StationID: "400"},
				{Channel: "7.2", StationID: "401"},
				{Channel: "7.3", StationID: "402"},
			},
			Stations: []schedulesdirect.Station{
				{StationID: "400", Callsign: "WFAA"},
				{StationID: "401", Callsign: "WFAA"},
				{StationID: "402", Callsign: "OTHER"},
			},
		}
		got := resolveGuideStationID(Channel{Number: "7.9", Callsign: "WFAA"}, dup)
		if got != "400" {
			t.Fatalf("got %q want .1 from filtered pool", got)
		}
	})

	t.Run("major with multiple entries prefers .1 without callsign", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "2.9", Callsign: "NOPE"}, detail)
		if got != "100" {
			t.Fatalf("got %q want 2.1 station", got)
		}
	})

	t.Run("major-only single map entry without callsign", func(t *testing.T) {
		got := resolveGuideStationID(Channel{Number: "5.9"}, detail)
		if got != "300" {
			t.Fatalf("got %q want major-only 300", got)
		}
	})

	t.Run("duplicate station ids in callsign list", func(t *testing.T) {
		dup := &schedulesdirect.LineupDetail{
			Map: []schedulesdirect.LineupMapEntry{
				{Channel: "11.1", StationID: "700"},
			},
			Stations: []schedulesdirect.Station{
				{StationID: "700", Callsign: "UNIQUE"},
				{StationID: "700", Callsign: "UNIQUE"},
			},
		}
		got := resolveGuideStationID(Channel{Number: "99.1", Callsign: "UNIQUE"}, dup)
		if got != "700" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestChannelMajorAndHelpers(t *testing.T) {
	if channelMajor("02.1") != "2" {
		t.Fatal(channelMajor("02.1"))
	}
	if channelMajor("5") != "5" {
		t.Fatal(channelMajor("5"))
	}
	if channelMajor(".1") != "0" {
		t.Fatal(channelMajor(".1"))
	}
	if min(1, 2) != 1 || min(3, 2) != 2 {
		t.Fatal("min")
	}
	if preferPrimarySubchannel([]string{"x"}, nil) != "" {
		t.Fatal("empty entries")
	}
	if stationCallsignMatches("", schedulesdirect.Station{Callsign: "ABC"}, true) {
		t.Fatal("empty want")
	}
	if stationCallsignMatches("AB", schedulesdirect.Station{Callsign: "ABCD"}, false) {
		t.Fatal("short prefix should not match")
	}
}
