package livetv

import (
	"strings"
	"testing"
)

func TestParseXMLTV(t *testing.T) {
	const sample = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="KABC">
    <display-name>7.1 KABC-HD</display-name>
    <icon src="https://example.test/kabc.png"/>
  </channel>
  <programme start="20260725190000 -0700" stop="20260725200000 -0700" channel="KABC">
    <title>Evening News</title>
    <sub-title>Weekend Edition</sub-title>
    <desc>Local and national headlines.</desc>
    <category>News</category>
    <episode-num system="xmltv_ns">1.4.</episode-num>
    <new/>
    <live/>
    <icon src="https://example.test/news.jpg"/>
  </programme>
</tv>`

	parsed, err := ParseXMLTV(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("ParseXMLTV() error = %v", err)
	}
	if len(parsed.Channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(parsed.Channels))
	}
	if parsed.Channels[0].ID != "KABC" || parsed.Channels[0].DisplayName != "7.1 KABC-HD" {
		t.Fatalf("unexpected channel: %+v", parsed.Channels[0])
	}
	if len(parsed.Programmes) != 1 {
		t.Fatalf("programmes = %d, want 1", len(parsed.Programmes))
	}
	programme := parsed.Programmes[0]
	if programme.ChannelID != "KABC" || programme.Title != "Evening News" || programme.Subtitle != "Weekend Edition" {
		t.Fatalf("unexpected programme text: %+v", programme)
	}
	if programme.Season == nil || *programme.Season != 2 || programme.Episode == nil || *programme.Episode != 5 {
		t.Fatalf("season/episode = %v/%v, want 2/5", programme.Season, programme.Episode)
	}
	if !programme.IsNew || !programme.IsLive {
		t.Fatalf("new/live flags not parsed: %+v", programme)
	}
	if got := programme.Start.Format("2006-01-02T15:04:05-07:00"); got != "2026-07-25T19:00:00-07:00" {
		t.Fatalf("start = %s", got)
	}
}
