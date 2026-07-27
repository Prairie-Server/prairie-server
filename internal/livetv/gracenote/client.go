// Package gracenote fetches TV listings from tvlistings.gracenote.com using the
// same public grid/provider endpoints that Zap2XML-style tools rely on.
package gracenote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL   = "https://tvlistings.gracenote.com"
	DefaultAssetsURL = "https://emby.tmsimg.com/assets/"
	DefaultUA        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36 Edg/137.0.3296.83"
	DefaultAffiliate = "orbebb"
	DefaultGridHours = 3
	DefaultLang      = "en"
)

// Client talks to the Gracenote TV listings web API.
type Client struct {
	BaseURL    string
	AssetsURL  string
	UserAgent  string
	Affiliate  string
	HTTPClient *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{
		BaseURL:    DefaultBaseURL,
		AssetsURL:  DefaultAssetsURL,
		UserAgent:  DefaultUA,
		Affiliate:  DefaultAffiliate,
		HTTPClient: httpClient,
	}
}

// Provider is a lineup option for a postal code.
type Provider struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	HeadendID string `json:"headendId"`
	LineupID  string `json:"lineupId"`
	Device    string `json:"device"`
}

// LineupOption is a UI-friendly provider row.
type LineupOption struct {
	Lineup    string `json:"lineup"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Location  string `json:"location"`
	Headend   string `json:"headend"`
	Device    string `json:"device"`
}

type providersResponse struct {
	Providers []Provider `json:"Providers"`
}

// Providers lists available lineups for a country + postal code.
func (c *Client) Providers(ctx context.Context, country, postalCode, lang string) ([]Provider, error) {
	country = strings.TrimSpace(country)
	if country == "" {
		country = "USA"
	}
	postalCode = strings.TrimSpace(postalCode)
	if postalCode == "" {
		return nil, fmt.Errorf("gracenote: postal code required")
	}
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = DefaultLang
	}
	path := fmt.Sprintf(
		"/gapzap_webapi/api/Providers/getPostalCodeProviders/%s/%s/gapzap/%s",
		url.PathEscape(country),
		url.PathEscape(postalCode),
		url.PathEscape(lang),
	)
	var out providersResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Providers, nil
}

// FlattenProviders converts providers into a UI-friendly list.
func FlattenProviders(providers []Provider) []LineupOption {
	out := make([]LineupOption, 0, len(providers))
	for _, p := range providers {
		out = append(out, LineupOption{
			Lineup:    strings.TrimSpace(p.LineupID),
			Name:      strings.TrimSpace(p.Name),
			Transport: strings.TrimSpace(p.Type),
			Location:  strings.TrimSpace(p.Location),
			Headend:   strings.TrimSpace(p.HeadendID),
			Device:    normalizeDevice(p.Device),
		})
	}
	return out
}

// GridParams identifies a lineup window for api/grid.
type GridParams struct {
	Country    string
	PostalCode string
	LineupID   string
	HeadendID  string
	Device     string
	Pref       string
}

// GridResponse is the top-level api/grid payload.
type GridResponse struct {
	Channels []Channel `json:"channels"`
}

// Channel is one station row in the grid.
type Channel struct {
	ChannelID string  `json:"channelId"`
	CallSign  string  `json:"callSign"`
	ChannelNo string  `json:"channelNo"`
	Thumbnail string  `json:"thumbnail"`
	Events    []Event `json:"events"`
}

// Event is one airing in the grid.
type Event struct {
	StartTime string   `json:"startTime"`
	EndTime   string   `json:"endTime"`
	Duration  string   `json:"duration"`
	Thumbnail string   `json:"thumbnail"`
	Flag      []string `json:"flag"`
	Filter    []string `json:"filter"`
	Tags      []string `json:"tags"`
	Rating    string   `json:"rating"`
	Program   Program  `json:"program"`
}

// Program is program metadata nested under an event.
// Season/Episode/ReleaseYear arrive as JSON strings or numbers depending on the airing.
type Program struct {
	ID           string `json:"id"`
	TMSID        string `json:"tmsId"`
	Title        string `json:"title"`
	ShortDesc    string `json:"shortDesc"`
	Season       *int   `json:"-"`
	Episode      *int   `json:"-"`
	EpisodeTitle string `json:"episodeTitle"`
	SeriesID     string `json:"seriesId"`
	ReleaseYear  *int   `json:"-"`
	IsGeneric    int    `json:"-"`
}

type programJSON struct {
	ID           string  `json:"id"`
	TMSID        string  `json:"tmsId"`
	Title        string  `json:"title"`
	ShortDesc    string  `json:"shortDesc"`
	Season       flexInt `json:"season"`
	Episode      flexInt `json:"episode"`
	EpisodeTitle string  `json:"episodeTitle"`
	SeriesID     string  `json:"seriesId"`
	ReleaseYear  flexInt `json:"releaseYear"`
	IsGeneric    flexInt `json:"isGeneric"`
}

func (p *Program) UnmarshalJSON(data []byte) error {
	var raw programJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.ID = raw.ID
	p.TMSID = raw.TMSID
	p.Title = raw.Title
	p.ShortDesc = raw.ShortDesc
	p.Season = raw.Season.Value
	p.Episode = raw.Episode.Value
	p.EpisodeTitle = raw.EpisodeTitle
	p.SeriesID = raw.SeriesID
	p.ReleaseYear = raw.ReleaseYear.Value
	if raw.IsGeneric.Value != nil {
		p.IsGeneric = *raw.IsGeneric.Value
	}
	return nil
}

// flexInt accepts JSON null, number, or numeric string.
type flexInt struct {
	Value *int
}

func (f *flexInt) UnmarshalJSON(data []byte) error {
	f.Value = nil
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		return nil
	}
	var asInt int
	if err := json.Unmarshal(data, &asInt); err == nil {
		f.Value = &asInt
		return nil
	}
	var asStr string
	if err := json.Unmarshal(data, &asStr); err != nil {
		return nil
	}
	asStr = strings.TrimSpace(asStr)
	if asStr == "" {
		return nil
	}
	n, err := strconv.Atoi(asStr)
	if err != nil {
		return nil
	}
	f.Value = &n
	return nil
}

// Grid fetches one timespan of listings (typically 3 hours).
func (c *Client) Grid(ctx context.Context, params GridParams, start time.Time, timespanHours int) (*GridResponse, error) {
	if timespanHours <= 0 {
		timespanHours = DefaultGridHours
	}
	lineupID := strings.TrimSpace(params.LineupID)
	postal := strings.TrimSpace(params.PostalCode)
	if lineupID == "" || postal == "" {
		return nil, fmt.Errorf("gracenote: lineupId and postalCode are required")
	}
	country := strings.TrimSpace(params.Country)
	if country == "" {
		country = "USA"
	}
	headend := strings.TrimSpace(params.HeadendID)
	if headend == "" {
		headend = headendFromLineup(lineupID)
	}
	device := normalizeDevice(params.Device)
	pref := strings.TrimSpace(params.Pref)
	if pref == "" {
		pref = "-"
	}
	aid := strings.TrimSpace(c.Affiliate)
	if aid == "" {
		aid = DefaultAffiliate
	}

	q := url.Values{}
	q.Set("time", strconv.FormatInt(start.UTC().Unix(), 10))
	q.Set("timespan", strconv.Itoa(timespanHours))
	q.Set("pref", pref)
	q.Set("lineupId", lineupID)
	q.Set("postalCode", postal)
	q.Set("country", country)
	q.Set("headendId", headend)
	q.Set("device", device)
	q.Set("aid", aid)
	q.Set("TMSID", "")
	q.Set("AffiliateID", aid)
	q.Set("FromPage", "TV Grid")
	q.Set("ActivityID", "1")
	q.Set("OVDID", "")
	q.Set("isOverride", "true")

	var out GridResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/grid?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AssetURL builds a program artwork URL from a grid thumbnail token.
func (c *Client) AssetURL(thumbnail string) string {
	thumbnail = strings.TrimSpace(thumbnail)
	if thumbnail == "" {
		return ""
	}
	if strings.HasPrefix(thumbnail, "http://") || strings.HasPrefix(thumbnail, "https://") {
		return thumbnail
	}
	base := strings.TrimSpace(c.AssetsURL)
	if base == "" {
		base = DefaultAssetsURL
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	if strings.HasSuffix(strings.ToLower(thumbnail), ".jpg") || strings.HasSuffix(strings.ToLower(thumbnail), ".png") {
		return base + thumbnail
	}
	return base + thumbnail + ".jpg"
}

// StationLogoURL normalizes a channel thumbnail (often protocol-relative).
func StationLogoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "?"); i >= 0 {
		raw = raw[:i]
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func normalizeDevice(device string) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return "-"
	}
	return device
}

// headendFromLineup derives headendId from lineup forms like USA-DISH623-DEFAULT
// or the OTA sentinel USA-lineupId-DEFAULT.
func headendFromLineup(lineupID string) string {
	parts := strings.Split(strings.TrimSpace(lineupID), "-")
	if len(parts) >= 3 && strings.EqualFold(parts[len(parts)-1], "DEFAULT") {
		return strings.Join(parts[1:len(parts)-1], "-")
	}
	if len(parts) >= 2 {
		return parts[1]
	}
	return lineupID
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	reqURL := base + path
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = strings.NewReader(string(buf))
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return err
	}
	ua := strings.TrimSpace(c.UserAgent)
	if ua == "" {
		ua = DefaultUA
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", base+"/")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return fmt.Errorf("gracenote: %s: %s", resp.Status, msg)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("gracenote: decode: %w", err)
	}
	return nil
}
