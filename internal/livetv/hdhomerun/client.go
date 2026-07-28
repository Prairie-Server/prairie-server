package hdhomerun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	http *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{http: httpClient}
}

type DeviceInfo struct {
	FriendlyName    string
	ModelNumber     string
	FirmwareVersion string
	TunerCount      int
	DeviceID        string
	BaseURL         string
	LineupURL       string
	// TranscodeCodecs lists the device-side transcode profiles the tuner
	// advertises ("heavy", "mobile", "internet480", …). Only the discontinued
	// EXTEND ever shipped them; current models omit the field and silently
	// ignore a ?transcode= query, so Prairie must never send one unless the
	// device says it is supported.
	TranscodeCodecs []string
}

type LineupChannel struct {
	GuideNumber string
	GuideName   string
	URL         string
	HD          bool
	Favorite    bool
}

type discoverJSON struct {
	FriendlyName    string          `json:"FriendlyName"`
	ModelNumber     string          `json:"ModelNumber"`
	FirmwareVersion string          `json:"FirmwareVersion"`
	TunerCount      int             `json:"TunerCount"`
	DeviceID        string          `json:"DeviceID"`
	BaseURL         string          `json:"BaseURL"`
	LineupURL       string          `json:"LineupURL"`
	TranscodeCodecs transcodeCodecs `json:"TranscodeCodecs"`
}

// transcodeCodecs accepts the shapes devices use for TranscodeCodecs: a JSON
// array on some firmware, a comma-separated string on others, and absent on
// every model that cannot transcode.
type transcodeCodecs []string

func (c *transcodeCodecs) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*c = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*c = normalizeCodecList(list)
		return nil
	}
	var joined string
	if err := json.Unmarshal(data, &joined); err != nil {
		return err
	}
	*c = normalizeCodecList(strings.Split(joined, ","))
	return nil
}

func normalizeCodecList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// lineupBool accepts HDHomeRun lineup flags that devices may emit as booleans
// or 0/1 integers (the common SiliconDust format).
type lineupBool bool

func (b *lineupBool) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*b = false
		return nil
	}

	var flag bool
	if err := json.Unmarshal(data, &flag); err == nil {
		*b = lineupBool(flag)
		return nil
	}

	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		i, err := num.Int64()
		if err != nil {
			return fmt.Errorf("lineup bool number: %w", err)
		}
		*b = lineupBool(i != 0)
		return nil
	}

	return fmt.Errorf("unsupported lineup bool %s", string(data))
}

type lineupJSON struct {
	// SiliconDust lineup.json fields used by Prairie:
	// GuideNumber is the channel's display number (for example "7.1").
	// GuideName is the callsign/display name shown by HDHomeRun clients.
	// URL is the MPEG-TS HTTP stream URL for the channel.
	// HD marks high-definition channels.
	// Favorite marks channels favorited on the HDHomeRun device; Prairie stores
	// all discovered channels and may use this as a future default filter.
	GuideNumber string     `json:"GuideNumber"`
	GuideName   string     `json:"GuideName"`
	URL         string     `json:"URL"`
	HD          lineupBool `json:"HD"`
	Favorite    lineupBool `json:"Favorite"`
}

func (c *Client) Discover(ctx context.Context, discoverURL, deviceID string) (*DeviceInfo, error) {
	endpoint, err := discoverEndpoint(discoverURL, deviceID)
	if err != nil {
		return nil, err
	}

	body, finalURL, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var info discoverJSON
	if err := json.Unmarshal(body, &info); err == nil && (info.DeviceID != "" || info.BaseURL != "" || info.ModelNumber != "") {
		return normalizeDeviceInfo(info, finalURL, deviceID), nil
	}

	var lineup []lineupJSON
	if err := json.Unmarshal(body, &lineup); err == nil {
		base := origin(finalURL)
		id := strings.TrimSpace(deviceID)
		if id == "" {
			id = hostWithoutPort(finalURL)
		}
		return &DeviceInfo{
			FriendlyName: "HDHomeRun",
			DeviceID:     id,
			BaseURL:      base,
			LineupURL:    strings.TrimRight(base, "/") + "/lineup.json",
			TunerCount:   1,
		}, nil
	}

	return nil, errors.New("hdhomerun discover: unsupported response")
}

func (c *Client) FetchLineup(ctx context.Context, baseURL string) ([]LineupChannel, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("hdhomerun lineup: base url is required")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/lineup.json"
	body, _, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var raw []lineupJSON
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("hdhomerun lineup: decode json: %w", err)
	}
	channels := make([]LineupChannel, 0, len(raw))
	for _, ch := range raw {
		channels = append(channels, LineupChannel{
			GuideNumber: strings.TrimSpace(ch.GuideNumber),
			GuideName:   strings.TrimSpace(ch.GuideName),
			URL:         strings.TrimSpace(ch.URL),
			HD:          bool(ch.HD),
			Favorite:    bool(ch.Favorite),
		})
	}
	return channels, nil
}

func (c *Client) get(ctx context.Context, endpoint string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("hdhomerun request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("hdhomerun get %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("hdhomerun get %s: status %d", endpoint, resp.StatusCode)
	}
	var v json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, "", fmt.Errorf("hdhomerun get %s: decode json: %w", endpoint, err)
	}
	return []byte(v), resp.Request.URL.String(), nil
}

func discoverEndpoint(discoverURL, deviceID string) (string, error) {
	if strings.TrimSpace(discoverURL) != "" {
		return strings.TrimSpace(discoverURL), nil
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", errors.New("hdhomerun discover: discover_url or device_id is required")
	}
	if strings.HasPrefix(deviceID, "http://") || strings.HasPrefix(deviceID, "https://") {
		return strings.TrimRight(deviceID, "/") + "/discover.json", nil
	}
	return "http://" + deviceID + "/discover.json", nil
}

func normalizeDeviceInfo(info discoverJSON, finalURL, fallbackDeviceID string) *DeviceInfo {
	base := strings.TrimSpace(info.BaseURL)
	if base == "" {
		base = origin(finalURL)
	}
	lineup := strings.TrimSpace(info.LineupURL)
	if lineup == "" && base != "" {
		lineup = strings.TrimRight(base, "/") + "/lineup.json"
	}
	id := strings.TrimSpace(info.DeviceID)
	if id == "" {
		id = strings.TrimSpace(fallbackDeviceID)
	}
	return &DeviceInfo{
		FriendlyName:    strings.TrimSpace(info.FriendlyName),
		ModelNumber:     strings.TrimSpace(info.ModelNumber),
		FirmwareVersion: strings.TrimSpace(info.FirmwareVersion),
		TunerCount:      info.TunerCount,
		DeviceID:        id,
		BaseURL:         base,
		LineupURL:       lineup,
		TranscodeCodecs: info.TranscodeCodecs,
	}
}

func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func hostWithoutPort(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return u.Hostname()
}
