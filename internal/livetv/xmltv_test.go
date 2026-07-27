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
	if parsed.Channels[0].IconURL != "https://example.test/kabc.png" {
		t.Fatalf("icon = %q", parsed.Channels[0].IconURL)
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

func TestParseXMLTVAlternateLayoutsAndEpisode(t *testing.T) {
	const sample = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="PBS"><display-name>PBS</display-name></channel>
  <programme start="20260725190000Z" stop="20260725200000+0000" channel="PBS">
    <title>Nova</title>
    <episode-num>S3E12</episode-num>
  </programme>
  <programme start="20260725210000" stop="20260725220000" channel="PBS">
    <title>Bare</title>
    <episode-num system="onscreen">E7</episode-num>
  </programme>
</tv>`

	parsed, err := ParseXMLTV(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("ParseXMLTV() error = %v", err)
	}
	if len(parsed.Programmes) != 2 {
		t.Fatalf("programmes = %d", len(parsed.Programmes))
	}
	if parsed.Programmes[0].Season == nil || *parsed.Programmes[0].Season != 3 {
		t.Fatalf("season = %v", parsed.Programmes[0].Season)
	}
	if parsed.Programmes[0].Episode == nil || *parsed.Programmes[0].Episode != 12 {
		t.Fatalf("episode = %v", parsed.Programmes[0].Episode)
	}
	if parsed.Programmes[1].Episode == nil || *parsed.Programmes[1].Episode != 7 {
		t.Fatalf("onscreen episode = %v", parsed.Programmes[1].Episode)
	}
}

func TestParseXMLTVErrors(t *testing.T) {
	if _, err := ParseXMLTV(strings.NewReader(`not xml`)); err == nil {
		t.Fatal("expected parse error")
	}
	const badTime = `<?xml version="1.0"?><tv><programme start="bad" stop="20260725200000" channel="x"><title>T</title></programme></tv>`
	if _, err := ParseXMLTV(strings.NewReader(badTime)); err == nil {
		t.Fatal("expected bad start time error")
	}
	const badStop = `<?xml version="1.0"?><tv><programme start="20260725190000" stop="bad" channel="x"><title>T</title></programme></tv>`
	if _, err := ParseXMLTV(strings.NewReader(badStop)); err == nil {
		t.Fatal("expected bad stop time error")
	}
}
