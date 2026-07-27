package livetv

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type XMLTV struct {
	Channels []XMLTVChannel
	Programs []XMLTVProgramme
}

type XMLTVChannel struct {
	ID          string
	DisplayName string
	IconURL     string
}

type XMLTVProgramme struct {
	ChannelID   string
	Start       time.Time
	Stop        time.Time
	Title       string
	Subtitle    string
	Description string
	Season      *int
	Episode     *int
	Genres      []string
	ImageURL    string
	IsNew       bool
	IsLive      bool
}

type xmltvDoc struct {
	Channels []xmltvChannel   `xml:"channel"`
	Programs []xmltvProgramme `xml:"program"`
}

type xmltvChannel struct {
	ID           string     `xml:"id,attr"`
	DisplayNames []string   `xml:"display-name"`
	Icon         *xmltvIcon `xml:"icon"`
}

type xmltvProgramme struct {
	Channel string         `xml:"channel,attr"`
	Start   string         `xml:"start,attr"`
	Stop    string         `xml:"stop,attr"`
	Titles  []string       `xml:"title"`
	Sub     []string       `xml:"sub-title"`
	Desc    []string       `xml:"desc"`
	Cats    []string       `xml:"category"`
	Episode []xmltvEpisode `xml:"episode-num"`
	Icon    *xmltvIcon     `xml:"icon"`
	New     *struct{}      `xml:"new"`
	Live    *struct{}      `xml:"live"`
}

type xmltvEpisode struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
}

type xmltvIcon struct {
	Src string `xml:"src,attr"`
}

func ParseXMLTV(r io.Reader) (*XMLTV, error) {
	var doc xmltvDoc
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse xmltv: %w", err)
	}

	out := &XMLTV{
		Channels: make([]XMLTVChannel, 0, len(doc.Channels)),
		Programs: make([]XMLTVProgramme, 0, len(doc.Programs)),
	}
	for _, ch := range doc.Channels {
		channel := XMLTVChannel{
			ID:          strings.TrimSpace(ch.ID),
			DisplayName: firstNonEmpty(ch.DisplayNames),
		}
		if ch.Icon != nil {
			channel.IconURL = strings.TrimSpace(ch.Icon.Src)
		}
		out.Channels = append(out.Channels, channel)
	}
	for _, p := range doc.Programs {
		start, err := parseXMLTVTime(p.Start)
		if err != nil {
			return nil, fmt.Errorf("program %q start: %w", p.Title(), err)
		}
		stop, err := parseXMLTVTime(p.Stop)
		if err != nil {
			return nil, fmt.Errorf("program %q stop: %w", p.Title(), err)
		}
		season, episode := parseXMLTVEpisode(p.Episode)
		program := XMLTVProgramme{
			ChannelID:   strings.TrimSpace(p.Channel),
			Start:       start,
			Stop:        stop,
			Title:       p.Title(),
			Subtitle:    firstNonEmpty(p.Sub),
			Description: firstNonEmpty(p.Desc),
			Season:      season,
			Episode:     episode,
			Genres:      trimStrings(p.Cats),
			IsNew:       p.New != nil,
			IsLive:      p.Live != nil,
		}
		if p.Icon != nil {
			program.ImageURL = strings.TrimSpace(p.Icon.Src)
		}
		out.Programs = append(out.Programs, program)
	}
	return out, nil
}

func (p xmltvProgramme) Title() string {
	return firstNonEmpty(p.Titles)
}

func parseXMLTVTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty xmltv time")
	}
	layouts := []string{
		"20060102150405 -0700",
		"20060102150405 -07:00",
		"20060102150405-0700",
		"20060102150405-07:00",
		"20060102150405Z0700",
		"20060102150405Z07:00",
		"20060102150405",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid xmltv time %q", value)
}

func parseXMLTVEpisode(values []xmltvEpisode) (*int, *int) {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v.System), "xmltv_ns") {
			return parseXMLTVNSEpisode(v.Value)
		}
	}
	for _, v := range values {
		if season, episode := parseEpisodeNumbers(v.Value); season != nil || episode != nil {
			return season, episode
		}
	}
	return nil, nil
}

func parseXMLTVNSEpisode(value string) (*int, *int) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	var season, episode *int
	if len(parts) > 0 {
		season = oneBasedXMLTVPart(parts[0])
	}
	if len(parts) > 1 {
		episode = oneBasedXMLTVPart(parts[1])
	}
	return season, episode
}

func oneBasedXMLTVPart(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	n++
	return &n
}

func parseEpisodeNumbers(value string) (*int, *int) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == 'S' || r == 's' || r == 'E' || r == 'e' || r == 'x' || r == 'X' || r == ' ' || r == '-'
	})
	nums := make([]int, 0, 2)
	for _, f := range fields {
		if f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err == nil {
			nums = append(nums, n)
		}
	}
	if len(nums) >= 2 {
		return &nums[0], &nums[1]
	}
	if len(nums) == 1 {
		return nil, &nums[0]
	}
	return nil, nil
}

func firstNonEmpty(values []string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}
