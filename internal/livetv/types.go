package livetv

import (
	"encoding/json"
	"time"
)

const (
	MaxGuideSources = 3

	TunerTypeHDHomeRun         = "hdhomerun"
	GuideSourceSchedulesDirect = "schedules_direct"
	// GuideSourceXMLSync is a native Zap2XML-style Gracenote listings sync.
	GuideSourceXMLSync = "xml_sync"

	// DefaultSchedulesDirectCountry is used when config omits country.
	DefaultSchedulesDirectCountry = "USA"
	// DefaultSchedulesDirectDays is how many days of schedule to pull per sync.
	DefaultSchedulesDirectDays = 7
	// DefaultXMLSyncCountry is used when XML sync config omits country.
	DefaultXMLSyncCountry = "USA"
	// DefaultXMLSyncDays is how many days of Gracenote grid to pull per sync.
	DefaultXMLSyncDays = 7
	// MaxXMLSyncDays caps Gracenote grid pull length.
	MaxXMLSyncDays = 14

	DiscoveredKindHDHomeRun   = "hdhomerun"
	DiscoveredKindDispatcharr = "dispatcharr"
)

// DiscoveredTuner is a candidate returned by admin discovery (not yet saved).
type DiscoveredTuner struct {
	Kind         string `json:"kind"`
	DeviceID     string `json:"device_id"`
	FriendlyName string `json:"friendly_name"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
	TunerCount   int    `json:"tuner_count"`
	DiscoverURL  string `json:"discover_url"`
	BaseURL      string `json:"base_url"`
	Source       string `json:"source"` // udp | probe
	AlreadyAdded bool   `json:"already_added"`
}

type DiscoverTunersRequest struct {
	// TimeoutMs bounds the UDP listen window (default 2000, max 10000).
	TimeoutMs int `json:"timeout_ms,omitempty"`
	// IncludeUDP enables SiliconDust HDHomeRun UDP broadcast discovery.
	IncludeUDP *bool `json:"include_udp,omitempty"`
	// ProbeURLs are Dispatcharr / HDHR base URLs to HTTP-probe for discover.json.
	ProbeURLs []string `json:"probe_urls,omitempty"`
}

// AddTunerInput adds an HDHomeRun-compatible tuner from a single address.
// Prefer URL. DiscoverURL and DeviceID remain accepted as legacy aliases for
// the same address (base URL, host, or discover.json path) — not the
// SiliconDust hardware id returned by discover.json.
type AddTunerInput struct {
	URL         string `json:"url"`
	DiscoverURL string `json:"discover_url"`
	DeviceID    string `json:"device_id"`
}

type DiscoverTunersResult struct {
	Candidates []DiscoveredTuner `json:"candidates"`
	Notes      []string          `json:"notes,omitempty"`
}

type Tuner struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	DeviceID     string     `json:"device_id"`
	DiscoverURL  string     `json:"discover_url"`
	BaseURL      string     `json:"base_url"`
	Model        string     `json:"model"`
	Firmware     string     `json:"firmware"`
	TunerCount   int        `json:"tuner_count"`
	Status       string     `json:"status"`
	ChannelCount int        `json:"channel_count"`
	LastError    string     `json:"last_error"`
	LastScanAt   *time.Time `json:"last_scan_at,omitempty"`
}

type Channel struct {
	ID             string  `json:"id"`
	TunerID        string  `json:"tuner_id"`
	Number         string  `json:"number"`
	NumberOverride *string `json:"number_override,omitempty"`
	Callsign       string  `json:"callsign"`
	Name           string  `json:"name"`
	LogoURL        string  `json:"logo_url"`
	HD             bool    `json:"hd"`
	Enabled        bool    `json:"enabled"`
	// StreamURL is the upstream tuner URL. MarshalJSON always emits stream_url
	// as an empty string so the /api/v1 field remains additive without leaking
	// private tuner addresses.
	StreamURL      string `json:"-"`
	GuideStationID string `json:"guide_station_id"`
}

// MarshalJSON keeps stream_url in the wire shape while redacting the upstream URL.
func (c Channel) MarshalJSON() ([]byte, error) {
	type Alias Channel
	return json.Marshal(struct {
		StreamURL string `json:"stream_url"`
		Alias
	}{StreamURL: "", Alias: Alias(c)})
}

type GuideSource struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	DisplayName string            `json:"display_name"`
	Config      map[string]string `json:"config"`
	Status      string            `json:"status"`
	LastError   string            `json:"last_error"`
	LastSyncAt  *time.Time        `json:"last_sync_at,omitempty"`
	NextSyncAt  *time.Time        `json:"next_sync_at,omitempty"`
}

// SchedulesDirectLineupsRequest looks up available lineups by postal code.
type SchedulesDirectLineupsRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	PasswordSHA1 string `json:"password_sha1"`
	Country      string `json:"country"`
	PostalCode   string `json:"postalcode"`
}

// XMLSyncLineupsRequest looks up Gracenote providers by postal code (no account).
type XMLSyncLineupsRequest struct {
	Country    string `json:"country"`
	PostalCode string `json:"postalcode"`
	Lang       string `json:"lang,omitempty"`
}

type Program struct {
	ID          string    `json:"id"`
	ChannelID   string    `json:"channel_id"`
	SourceID    string    `json:"source_id,omitempty"`
	SeriesID    string    `json:"series_id"`
	ExternalID  string    `json:"external_id,omitempty"`
	Start       time.Time `json:"start"`
	Stop        time.Time `json:"stop"`
	Title       string    `json:"title"`
	Subtitle    string    `json:"subtitle"`
	Description string    `json:"description"`
	Season      *int      `json:"season,omitempty"`
	Episode     *int      `json:"episode,omitempty"`
	Genres      []string  `json:"genres"`
	ImageURL    string    `json:"image_url"`
	IsNew       bool      `json:"is_new"`
	IsLive      bool      `json:"is_live"`
}

type LiveSession struct {
	ID                string `json:"id"`
	ChannelID         string `json:"channel_id"`
	TunerID           string `json:"tuner_id"`
	TunerIndex        int    `json:"tuner_index"`
	UserID            int    `json:"user_id,omitempty"`
	ProfileID         string `json:"profile_id,omitempty"`
	PlaybackSessionID string `json:"playback_session_id,omitempty"`
	Status            string `json:"status"`
	HLSURL            string `json:"hls_url,omitempty"`
	// StreamURL may hold an upstream URL in memory; MarshalJSON emits the
	// authenticated proxy path (or empty) so clients never receive tuner URLs.
	StreamURL string `json:"-"`
	// Transport is "mpegts" for the session proxy or "hls" when a playback
	// bridge remuxes into the normal Prairie player pipeline.
	Transport  string     `json:"transport,omitempty"`
	Note       string     `json:"note,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

// MarshalJSON preserves stream_url for /api/v1 clients using the public proxy path.
func (s LiveSession) MarshalJSON() ([]byte, error) {
	publicStream := ""
	if s.ID != "" {
		publicStream = PublicSessionStreamPath(s.ID)
	}
	hls := s.HLSURL
	if !IsClientSafePlayURL(hls) {
		hls = publicStream
	}
	type Alias LiveSession
	return json.Marshal(struct {
		HLSURL    string `json:"hls_url,omitempty"`
		StreamURL string `json:"stream_url"`
		Alias
	}{
		HLSURL:    hls,
		StreamURL: publicStream,
		Alias:     Alias(s),
	})
}

type Recording struct {
	ID            string    `json:"id"`
	ProgramID     string    `json:"program_id,omitempty"`
	ChannelID     string    `json:"channel_id"`
	SeriesRuleID  string    `json:"series_rule_id,omitempty"`
	UserID        int       `json:"user_id,omitempty"`
	ProfileID     string    `json:"profile_id,omitempty"`
	Status        string    `json:"status"`
	Path          string    `json:"path,omitempty"`
	LibraryItemID string    `json:"library_item_id,omitempty"`
	Start         time.Time `json:"start"`
	Stop          time.Time `json:"stop"`
	Title         string    `json:"title"`
	LastError     string    `json:"last_error,omitempty"`
}

type SeriesRule struct {
	ID         string  `json:"id"`
	SeriesID   string  `json:"series_id"`
	ChannelID  *string `json:"channel_id,omitempty"`
	UserID     int     `json:"user_id,omitempty"`
	ProfileID  string  `json:"profile_id,omitempty"`
	TitleMatch string  `json:"title_match"`
	NewOnly    bool    `json:"new_only"`
	KeepLast   int     `json:"keep_last"`
	Enabled    bool    `json:"enabled"`
}

type ChannelPatch struct {
	Enabled        *bool
	NumberOverride *string
	GuideStationID *string
}

type SessionCreate struct {
	ChannelID         string
	TunerID           string
	TunerIndex        int
	UserID            int
	ProfileID         string
	PlaybackSessionID string
}
