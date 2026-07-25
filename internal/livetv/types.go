package livetv

import "time"

const (
	MaxGuideSources = 3

	TunerTypeHDHomeRun        = "hdhomerun"
	GuideSourceXMLTVURL       = "xmltv_url"
	GuideSourceSchedulesDirect = "schedules_direct"
)

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
	StreamURL      string  `json:"stream_url"`
	GuideStationID string  `json:"guide_station_id"`
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
	ID                string     `json:"id"`
	ChannelID         string     `json:"channel_id"`
	TunerID           string     `json:"tuner_id"`
	TunerIndex        int        `json:"tuner_index"`
	UserID            int        `json:"user_id,omitempty"`
	ProfileID         string     `json:"profile_id,omitempty"`
	PlaybackSessionID string     `json:"playback_session_id,omitempty"`
	Status            string     `json:"status"`
	HLSURL            string     `json:"hls_url,omitempty"`
	StreamURL         string     `json:"stream_url,omitempty"`
	Note              string     `json:"note,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ReleasedAt        *time.Time `json:"released_at,omitempty"`
}

type Recording struct {
	ID             string    `json:"id"`
	ProgramID      string    `json:"program_id,omitempty"`
	ChannelID      string    `json:"channel_id"`
	SeriesRuleID   string    `json:"series_rule_id,omitempty"`
	Status         string    `json:"status"`
	Path           string    `json:"path,omitempty"`
	LibraryItemID  string    `json:"library_item_id,omitempty"`
	Start          time.Time `json:"start"`
	Stop           time.Time `json:"stop"`
	Title          string    `json:"title"`
	LastError      string    `json:"last_error,omitempty"`
}

type SeriesRule struct {
	ID         string  `json:"id"`
	SeriesID   string  `json:"series_id"`
	ChannelID  *string `json:"channel_id,omitempty"`
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
