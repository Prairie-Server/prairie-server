// Package schedulesdirect talks to the Schedules Direct JSON API (20141201).
package schedulesdirect

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://json.schedulesdirect.org/20141201"
	DefaultUA      = "Prairie/1.0 (Live TV guide; https://github.com/Prairie-Server/prairie-server)"
	maxBatch       = 5000
)

// Client is a Schedules Direct JSON API client.
type Client struct {
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{
		BaseURL:    DefaultBaseURL,
		UserAgent:  DefaultUA,
		HTTPClient: httpClient,
	}
}

// HashPassword returns the lowercase SHA-1 hex digest Schedules Direct expects.
func HashPassword(password string) string {
	sum := sha1.Sum([]byte(password))
	return hex.EncodeToString(sum[:])
}

// APIError is a Schedules Direct JSON API error payload.
type APIError struct {
	Response string `json:"response"`
	Code     int    `json:"code"`
	Message  string `json:"message"`
}

func (e APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("schedules direct: %s (code %d)", e.Message, e.Code)
	}
	if e.Response != "" {
		return fmt.Sprintf("schedules direct: %s (code %d)", e.Response, e.Code)
	}
	return fmt.Sprintf("schedules direct: code %d", e.Code)
}

type tokenResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

// Token authenticates and returns a session token.
func (c *Client) Token(ctx context.Context, username, passwordSHA1 string) (string, error) {
	body := map[string]string{
		"username": strings.TrimSpace(username),
		"password": strings.ToLower(strings.TrimSpace(passwordSHA1)),
	}
	var out tokenResponse
	if err := c.doJSON(ctx, http.MethodPost, "/token", "", body, &out); err != nil {
		return "", err
	}
	if out.Code != 0 || strings.TrimSpace(out.Token) == "" {
		return "", APIError{Code: out.Code, Message: out.Message, Response: "TOKEN_FAILED"}
	}
	return out.Token, nil
}

type Headend struct {
	Headend   string          `json:"headend"`
	Transport string          `json:"transport"`
	Location  string          `json:"location"`
	Lineups   []HeadendLineup `json:"lineups"`
}

type HeadendLineup struct {
	Name   string `json:"name"`
	Lineup string `json:"lineup"`
	URI    string `json:"uri"`
}

// LineupOption is a flattened lineup choice for admin UI.
type LineupOption struct {
	Lineup    string `json:"lineup"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Location  string `json:"location"`
	Headend   string `json:"headend"`
}

// Headends lists lineups available for a country + postal code.
func (c *Client) Headends(ctx context.Context, token, country, postalCode string) ([]Headend, error) {
	country = strings.TrimSpace(country)
	if country == "" {
		country = "USA"
	}
	q := url.Values{}
	q.Set("country", country)
	q.Set("postalcode", strings.TrimSpace(postalCode))
	path := "/headends?" + q.Encode()
	var out []Headend
	if err := c.doJSON(ctx, http.MethodGet, path, token, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// FlattenLineups converts headends into a UI-friendly list.
func FlattenLineups(headends []Headend) []LineupOption {
	out := make([]LineupOption, 0)
	for _, h := range headends {
		for _, lu := range h.Lineups {
			out = append(out, LineupOption{
				Lineup:    lu.Lineup,
				Name:      lu.Name,
				Transport: h.Transport,
				Location:  h.Location,
				Headend:   h.Headend,
			})
		}
	}
	return out
}

type statusResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// AddLineup subscribes the account to a lineup. Duplicate lineups are ignored.
func (c *Client) AddLineup(ctx context.Context, token, lineupID string) error {
	lineupID = strings.TrimSpace(lineupID)
	if lineupID == "" {
		return fmt.Errorf("schedules direct: lineup id required")
	}
	var out statusResponse
	err := c.doJSON(ctx, http.MethodPut, "/lineups/"+url.PathEscape(lineupID), token, nil, &out)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "already") || strings.Contains(msg, "duplicate") {
			return nil
		}
		return err
	}
	if out.Code != 0 {
		msg := strings.ToLower(out.Message)
		if strings.Contains(msg, "already") || strings.Contains(msg, "duplicate") {
			return nil
		}
		return APIError{Code: out.Code, Message: out.Message}
	}
	return nil
}

type LineupDetail struct {
	Map      []LineupMapEntry `json:"map"`
	Stations []Station        `json:"stations"`
}

type LineupMapEntry struct {
	Channel   string `json:"channel"`
	StationID string `json:"stationID"`
}

type Station struct {
	StationID string `json:"stationID"`
	Name      string `json:"name"`
	Callsign  string `json:"callsign"`
	Affiliate string `json:"affiliate"`
}

// Lineup downloads channel → station mapping for a subscribed lineup.
func (c *Client) Lineup(ctx context.Context, token, lineupID string) (*LineupDetail, error) {
	lineupID = strings.TrimSpace(lineupID)
	var out LineupDetail
	if err := c.doJSON(ctx, http.MethodGet, "/lineups/"+url.PathEscape(lineupID), token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type ScheduleRequest struct {
	StationID string   `json:"stationID"`
	Date      []string `json:"date,omitempty"`
}

type StationSchedule struct {
	StationID string            `json:"stationID"`
	Programs  []ScheduleProgram `json:"programs"`
	Code      int               `json:"code"`
	Message   string            `json:"message"`
}

type ScheduleProgram struct {
	ProgramID   string `json:"programID"`
	AirDateTime string `json:"airDateTime"`
	Duration    int    `json:"duration"`
	MD5         string `json:"md5"`
	New         *bool  `json:"new"`
	Live        *bool  `json:"live"`
}

// Schedules downloads schedules for station IDs (batched).
func (c *Client) Schedules(ctx context.Context, token string, reqs []ScheduleRequest) ([]StationSchedule, error) {
	out := make([]StationSchedule, 0, len(reqs))
	for i := 0; i < len(reqs); i += maxBatch {
		end := i + maxBatch
		if end > len(reqs) {
			end = len(reqs)
		}
		var batch []StationSchedule
		if err := c.doJSON(ctx, http.MethodPost, "/schedules", token, reqs[i:end], &batch); err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

type ProgramDetail struct {
	ProgramID       string    `json:"programID"`
	Titles          []Title   `json:"titles"`
	EpisodeTitle150 string    `json:"episodeTitle150"`
	Descriptions    DescBlock `json:"descriptions"`
	Genres          []string  `json:"genres"`
	Metadata        []map[string]struct {
		Season  int `json:"season"`
		Episode int `json:"episode"`
	} `json:"metadata"`
}

type Title struct {
	Title120 string `json:"title120"`
}

type DescBlock struct {
	Description1000 []LangText `json:"description1000"`
	Description100  []LangText `json:"description100"`
}

type LangText struct {
	Description string `json:"description"`
}

// Programs downloads program metadata for IDs (batched).
func (c *Client) Programs(ctx context.Context, token string, ids []string) ([]ProgramDetail, error) {
	out := make([]ProgramDetail, 0, len(ids))
	for i := 0; i < len(ids); i += maxBatch {
		end := i + maxBatch
		if end > len(ids) {
			end = len(ids)
		}
		var batch []ProgramDetail
		if err := c.doJSON(ctx, http.MethodPost, "/programs", token, ids[i:end], &batch); err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (p ProgramDetail) Title() string {
	for _, t := range p.Titles {
		if strings.TrimSpace(t.Title120) != "" {
			return strings.TrimSpace(t.Title120)
		}
	}
	return ""
}

func (p ProgramDetail) Description() string {
	for _, d := range p.Descriptions.Description1000 {
		if strings.TrimSpace(d.Description) != "" {
			return strings.TrimSpace(d.Description)
		}
	}
	for _, d := range p.Descriptions.Description100 {
		if strings.TrimSpace(d.Description) != "" {
			return strings.TrimSpace(d.Description)
		}
	}
	return ""
}

func (p ProgramDetail) SeasonEpisode() (season, episode *int) {
	for _, block := range p.Metadata {
		for _, meta := range block {
			if meta.Season > 0 {
				s := meta.Season
				season = &s
			}
			if meta.Episode > 0 {
				e := meta.Episode
				episode = &e
			}
			if season != nil || episode != nil {
				return season, episode
			}
		}
	}
	return nil, nil
}

func (c *Client) doJSON(ctx context.Context, method, path, token string, body any, out any) error {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = DefaultUA
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	if token != "" {
		req.Header.Set("token", token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("schedules direct request: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 64<<20)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("schedules direct read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Code != 0 || apiErr.Message != "" || apiErr.Response != "" {
			return apiErr
		}
		return fmt.Errorf("schedules direct: status %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		// Some error payloads still return HTTP 200 with a code field.
		var apiErr APIError
		if json.Unmarshal(raw, &apiErr) == nil && apiErr.Code != 0 {
			return apiErr
		}
		return fmt.Errorf("schedules direct decode: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
