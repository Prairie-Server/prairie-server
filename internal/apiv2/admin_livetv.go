package apiv2

import (
	"context"
	"net/http"
	"time"

	"github.com/prairie-server/prairie-server/internal/api/handlers"
	"github.com/prairie-server/prairie-server/internal/livetv"
	"github.com/prairie-server/prairie-server/internal/livetv/gracenote"
	"github.com/prairie-server/prairie-server/internal/livetv/schedulesdirect"
)

const adminLiveTVTag = "admin-livetv"

// LiveTVAdminService is the administrator half of the Live TV seam: tuners,
// channel edits and guide sources. *handlers.LiveTVHandler implements it.
type LiveTVAdminService interface {
	ListTuners(context.Context) ([]livetv.Tuner, error)
	DiscoverTuners(context.Context, livetv.DiscoverTunersRequest) (*livetv.DiscoverTunersResult, error)
	AddTuner(context.Context, livetv.AddTunerInput) (*livetv.Tuner, error)
	DeleteTuner(context.Context, string) error
	ScanTuner(context.Context, string) (*livetv.Tuner, error)
	PatchChannel(context.Context, string, livetv.ChannelPatch) (*livetv.Channel, error)
	ListGuideSources(context.Context) ([]livetv.GuideSource, error)
	LookupSchedulesDirectLineups(context.Context, livetv.SchedulesDirectLineupsRequest) ([]schedulesdirect.LineupOption, error)
	LookupXMLSyncLineups(context.Context, livetv.XMLSyncLineupsRequest) ([]gracenote.LineupOption, error)
	CreateGuideSource(context.Context, *livetv.GuideSource) (*livetv.GuideSource, error)
	UpdateGuideSource(context.Context, string, handlers.GuideSourcePatch) (*livetv.GuideSource, error)
	DeleteGuideSource(context.Context, string) error
	SyncGuideSource(context.Context, string) (*livetv.GuideSource, error)
}

type LiveTVTuner struct {
	ID              ID       `json:"id"`
	Type            string   `json:"type"`
	DeviceID        string   `json:"device_id"`
	DiscoverURL     string   `json:"discover_url"`
	BaseURL         string   `json:"base_url"`
	Model           string   `json:"model"`
	Firmware        string   `json:"firmware"`
	TunerCount      int      `json:"tuner_count"`
	Status          string   `json:"status"`
	ChannelCount    int      `json:"channel_count"`
	LastError       string   `json:"last_error"`
	LastScanAt      *Instant `json:"last_scan_at,omitempty"`
	TranscodeCodecs []string `json:"transcode_codecs" doc:"Device-side transcode profiles; empty on every current model"`
}

type LiveTVDiscoveredTuner struct {
	Kind         string `json:"kind" doc:"hdhomerun or dispatcharr"`
	DeviceID     string `json:"device_id"`
	FriendlyName string `json:"friendly_name"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
	TunerCount   int    `json:"tuner_count"`
	DiscoverURL  string `json:"discover_url"`
	BaseURL      string `json:"base_url"`
	Source       string `json:"source" doc:"udp or probe"`
	AlreadyAdded bool   `json:"already_added"`
}

type LiveTVTunerDiscovery struct {
	Items []LiveTVDiscoveredTuner `json:"items" doc:"Candidates not yet saved; empty, never null"`
	Notes []string                `json:"notes"`
}

type LiveTVTunerDiscoveryBody struct {
	TimeoutMs  int      `json:"timeout_ms,omitempty" minimum:"0" maximum:"10000" doc:"UDP listen window; default 2000"`
	IncludeUDP *bool    `json:"include_udp,omitempty" nullable:"false" doc:"HDHomeRun UDP broadcast discovery; default true"`
	ProbeURLs  []string `json:"probe_urls,omitempty" maxItems:"16" doc:"Dispatcharr or HDHomeRun base URLs to probe for discover.json"`
}

type LiveTVTunerBody struct {
	URL string `json:"url" minLength:"1" maxLength:"2048" doc:"Base URL, host, or discover.json URL of an HDHomeRun-compatible tuner"`
}

type LiveTVChannelUpdate struct {
	Enabled        *bool   `json:"enabled,omitempty" nullable:"false"`
	NumberOverride *string `json:"number_override,omitempty" nullable:"false" maxLength:"32" doc:"Empty clears the override"`
	GuideStationID *string `json:"guide_station_id,omitempty" nullable:"false" maxLength:"256"`
}

type LiveTVGuideSource struct {
	ID          ID                `json:"id"`
	Type        string            `json:"type" doc:"schedules_direct or xml_sync"`
	Priority    int               `json:"priority"`
	Enabled     bool              `json:"enabled"`
	DisplayName string            `json:"display_name"`
	Config      map[string]string `json:"config" doc:"Provider settings; secrets are redacted"`
	Status      string            `json:"status"`
	LastError   string            `json:"last_error"`
	LastSyncAt  *Instant          `json:"last_sync_at,omitempty"`
	NextSyncAt  *Instant          `json:"next_sync_at,omitempty"`
}

type LiveTVGuideSourceBody struct {
	Type        string            `json:"type" enum:"schedules_direct,xml_sync"`
	Priority    int               `json:"priority,omitempty"`
	Enabled     bool              `json:"enabled,omitempty"`
	DisplayName string            `json:"display_name,omitempty" maxLength:"256"`
	Config      map[string]string `json:"config,omitempty" maxProperties:"32"`
}

type LiveTVGuideSourceUpdate struct {
	Type        *string            `json:"type,omitempty" nullable:"false" enum:"schedules_direct,xml_sync"`
	Priority    *int               `json:"priority,omitempty" nullable:"false"`
	Enabled     *bool              `json:"enabled,omitempty" nullable:"false"`
	DisplayName *string            `json:"display_name,omitempty" nullable:"false" maxLength:"256"`
	Config      *map[string]string `json:"config,omitempty" nullable:"false" maxProperties:"32" doc:"Replaces the whole config; a redacted secret sent back unchanged keeps the stored value"`
}

type LiveTVLineup struct {
	Lineup    string `json:"lineup"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Location  string `json:"location"`
	Headend   string `json:"headend"`
	Device    string `json:"device,omitempty"`
}

type LiveTVSchedulesDirectLineupsBody struct {
	Username     string `json:"username" minLength:"1" maxLength:"256"`
	Password     string `json:"password,omitempty" maxLength:"256"`
	PasswordSHA1 string `json:"password_sha1,omitempty" maxLength:"64"`
	Country      string `json:"country,omitempty" maxLength:"8"`
	PostalCode   string `json:"postal_code" minLength:"1" maxLength:"32"`
}

type LiveTVXMLSyncLineupsBody struct {
	Country    string `json:"country,omitempty" maxLength:"8"`
	PostalCode string `json:"postal_code" minLength:"1" maxLength:"32"`
	Lang       string `json:"lang,omitempty" maxLength:"16"`
}

type AdminLiveTVTunersOutput struct{ Body Collection[LiveTVTuner] }
type AdminLiveTVTunerOutput struct{ Body LiveTVTuner }
type AdminLiveTVTunerInput struct {
	TunerID ID `path:"tuner_id" minLength:"1"`
}
type AdminLiveTVTunerCreateInput struct{ Body LiveTVTunerBody }
type AdminLiveTVDiscoverInput struct {
	Body *LiveTVTunerDiscoveryBody `required:"false" doc:"Absent runs UDP discovery with the default window"`
}
type AdminLiveTVDiscoverOutput struct{ Body LiveTVTunerDiscovery }
type AdminLiveTVChannelUpdateInput struct {
	ChannelID ID `path:"channel_id" minLength:"1"`
	Body      LiveTVChannelUpdate
}
type AdminLiveTVChannelOutput struct{ Body LiveTVChannel }
type AdminLiveTVGuideSourcesOutput struct{ Body Collection[LiveTVGuideSource] }
type AdminLiveTVGuideSourceOutput struct{ Body LiveTVGuideSource }
type AdminLiveTVGuideSourceInput struct {
	SourceID ID `path:"source_id" minLength:"1"`
}
type AdminLiveTVGuideSourceCreateInput struct{ Body LiveTVGuideSourceBody }
type AdminLiveTVGuideSourceUpdateInput struct {
	SourceID ID `path:"source_id" minLength:"1"`
	Body     LiveTVGuideSourceUpdate
}
type AdminLiveTVLineupsOutput struct{ Body Collection[LiveTVLineup] }
type AdminLiveTVSchedulesDirectLineupsInput struct {
	Body LiveTVSchedulesDirectLineupsBody
}
type AdminLiveTVXMLSyncLineupsInput struct{ Body LiveTVXMLSyncLineupsBody }

func registerAdminLiveTV(reg *Registry) {
	op := func(method, path, id string, retry RetrySafety, summary string) Operation {
		o := humaOp(method, Prefix+"/admin/livetv"+path, id, adminLiveTVTag, summary)
		o.Errors = []int{http.StatusConflict, http.StatusNotFound}
		operation := Operation{Operation: o, Class: ClassActingAdmin, ServiceBacked: true, RetrySafety: retry}
		if method != http.MethodGet {
			operation.DemoRestricted = true
			operation.Errors = append(operation.Errors, http.StatusUnprocessableEntity, http.StatusNotImplemented)
		}
		return operation
	}
	Register(reg, op(http.MethodGet, "/tuners", "listAdminLiveTVTuners", "", "List configured Live TV tuners."), func(ctx context.Context, _ *struct{}) (*AdminLiveTVTunersOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		tuners, err := svc.ListTuners(ctx)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVTunersOutput{Body: NewCollection(mapSlice(tuners, liveTVTunerOf))}, nil
	})
	add := op(http.MethodPost, "/tuners", "addAdminLiveTVTuner", RetrySafetyUniqueConstraint, "Add an HDHomeRun-compatible tuner by address and scan its lineup. Adding a tuner that is already configured returns it.")
	add.DefaultStatus = http.StatusCreated
	Register(reg, add, func(ctx context.Context, in *AdminLiveTVTunerCreateInput) (*AdminLiveTVTunerOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		tuner, err := svc.AddTuner(ctx, livetv.AddTunerInput{URL: in.Body.URL})
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVTunerOutput{Body: liveTVTunerOf(*tuner)}, nil
	})
	Register(reg, op(http.MethodPost, "/tuners/discover", "discoverAdminLiveTVTuners", RetrySafetyNaturalIdempotent, "Discover tuners on the network (HDHomeRun UDP broadcast and URL probes) without saving them."), func(ctx context.Context, in *AdminLiveTVDiscoverInput) (*AdminLiveTVDiscoverOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		var req livetv.DiscoverTunersRequest
		if in.Body != nil {
			req = livetv.DiscoverTunersRequest{TimeoutMs: in.Body.TimeoutMs, IncludeUDP: in.Body.IncludeUDP, ProbeURLs: in.Body.ProbeURLs}
		}
		result, err := svc.DiscoverTuners(ctx, req)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		body := LiveTVTunerDiscovery{Items: []LiveTVDiscoveredTuner{}, Notes: []string{}}
		if result != nil {
			body.Items = mapSlice(result.Candidates, func(c livetv.DiscoveredTuner) LiveTVDiscoveredTuner {
				return LiveTVDiscoveredTuner(c)
			})
			body.Notes = append(body.Notes, result.Notes...)
		}
		return &AdminLiveTVDiscoverOutput{Body: body}, nil
	})
	del := op(http.MethodDelete, "/tuners/{tuner_id}", "deleteAdminLiveTVTuner", RetrySafetyNaturalIdempotent, "Remove a tuner and its channels.")
	del.DefaultStatus = http.StatusNoContent
	Register(reg, del, func(ctx context.Context, in *AdminLiveTVTunerInput) (*struct{}, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		if err := svc.DeleteTuner(ctx, string(in.TunerID)); err != nil {
			return nil, liveTVProblem(err)
		}
		return &struct{}{}, nil
	})
	Register(reg, op(http.MethodPost, "/tuners/{tuner_id}/scan", "scanAdminLiveTVTuner", RetrySafetyNaturalIdempotent, "Rescan a tuner's channel lineup now and return the refreshed tuner."), func(ctx context.Context, in *AdminLiveTVTunerInput) (*AdminLiveTVTunerOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		tuner, err := svc.ScanTuner(ctx, string(in.TunerID))
		if err != nil {
			return nil, liveTVProblem(err)
		}
		if tuner == nil {
			return nil, liveTVProblem(livetv.ErrNotFound)
		}
		return &AdminLiveTVTunerOutput{Body: liveTVTunerOf(*tuner)}, nil
	})
	Register(reg, op(http.MethodPatch, "/channels/{channel_id}", "updateAdminLiveTVChannel", RetrySafetyNaturalIdempotent, "Enable or disable a channel, override its number, or map it to a guide station. Omitted members are unchanged."), func(ctx context.Context, in *AdminLiveTVChannelUpdateInput) (*AdminLiveTVChannelOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		channel, err := svc.PatchChannel(ctx, string(in.ChannelID), livetv.ChannelPatch{Enabled: b.Enabled, NumberOverride: b.NumberOverride, GuideStationID: b.GuideStationID})
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVChannelOutput{Body: liveTVChannelOf(*channel)}, nil
	})
	registerAdminLiveTVGuideSources(reg, op)
}

func registerAdminLiveTVGuideSources(reg *Registry, op func(method, path, id string, retry RetrySafety, summary string) Operation) {
	Register(reg, op(http.MethodGet, "/guide-sources", "listAdminLiveTVGuideSources", "", "List guide data sources in priority order; secrets are redacted."), func(ctx context.Context, _ *struct{}) (*AdminLiveTVGuideSourcesOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		sources, err := svc.ListGuideSources(ctx)
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVGuideSourcesOutput{Body: NewCollection(mapSlice(sources, liveTVGuideSourceOf))}, nil
	})
	create := op(http.MethodPost, "/guide-sources", "createAdminLiveTVGuideSource", RetrySafetyNonRetryable, "Add a guide data source (Schedules Direct or Gracenote XML sync). At most three may be enabled. Never retried automatically.")
	create.DefaultStatus = http.StatusCreated
	Register(reg, create, func(ctx context.Context, in *AdminLiveTVGuideSourceCreateInput) (*AdminLiveTVGuideSourceOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		created, err := svc.CreateGuideSource(ctx, &livetv.GuideSource{Type: b.Type, Priority: b.Priority, Enabled: b.Enabled, DisplayName: b.DisplayName, Config: b.Config})
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVGuideSourceOutput{Body: liveTVGuideSourceOf(*created)}, nil
	})
	Register(reg, op(http.MethodPatch, "/guide-sources/{source_id}", "updateAdminLiveTVGuideSource", RetrySafetyNaturalIdempotent, "Update a guide source. Omitted members are unchanged; config replaces the whole map."), func(ctx context.Context, in *AdminLiveTVGuideSourceUpdateInput) (*AdminLiveTVGuideSourceOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		updated, err := svc.UpdateGuideSource(ctx, string(in.SourceID), handlers.GuideSourcePatch{Type: b.Type, Priority: b.Priority, Enabled: b.Enabled, DisplayName: b.DisplayName, Config: b.Config})
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVGuideSourceOutput{Body: liveTVGuideSourceOf(*updated)}, nil
	})
	del := op(http.MethodDelete, "/guide-sources/{source_id}", "deleteAdminLiveTVGuideSource", RetrySafetyNaturalIdempotent, "Remove a guide source.")
	del.DefaultStatus = http.StatusNoContent
	Register(reg, del, func(ctx context.Context, in *AdminLiveTVGuideSourceInput) (*struct{}, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		if err := svc.DeleteGuideSource(ctx, string(in.SourceID)); err != nil {
			return nil, liveTVProblem(err)
		}
		return &struct{}{}, nil
	})
	Register(reg, op(http.MethodPost, "/guide-sources/{source_id}/sync", "syncAdminLiveTVGuideSource", RetrySafetyNaturalIdempotent, "Sync a guide source now and return it with its refreshed sync state."), func(ctx context.Context, in *AdminLiveTVGuideSourceInput) (*AdminLiveTVGuideSourceOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		source, err := svc.SyncGuideSource(ctx, string(in.SourceID))
		if err != nil {
			return nil, liveTVProblem(err)
		}
		if source == nil {
			return nil, liveTVProblem(livetv.ErrNotFound)
		}
		return &AdminLiveTVGuideSourceOutput{Body: liveTVGuideSourceOf(*source)}, nil
	})
	Register(reg, op(http.MethodPost, "/schedules-direct/lineups", "lookupAdminLiveTVSchedulesDirectLineups", RetrySafetyNaturalIdempotent, "Look up the Schedules Direct lineups available at a postal code with the given account. Nothing is saved."), func(ctx context.Context, in *AdminLiveTVSchedulesDirectLineupsInput) (*AdminLiveTVLineupsOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		lineups, err := svc.LookupSchedulesDirectLineups(ctx, livetv.SchedulesDirectLineupsRequest{Username: b.Username, Password: b.Password, PasswordSHA1: b.PasswordSHA1, Country: b.Country, PostalCode: b.PostalCode})
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVLineupsOutput{Body: NewCollection(mapSlice(lineups, func(l schedulesdirect.LineupOption) LiveTVLineup {
			return LiveTVLineup{Lineup: l.Lineup, Name: l.Name, Transport: l.Transport, Location: l.Location, Headend: l.Headend}
		}))}, nil
	})
	Register(reg, op(http.MethodPost, "/xml-sync/lineups", "lookupAdminLiveTVXMLSyncLineups", RetrySafetyNaturalIdempotent, "Look up the Gracenote providers available at a postal code; no account is needed. Nothing is saved."), func(ctx context.Context, in *AdminLiveTVXMLSyncLineupsInput) (*AdminLiveTVLineupsOutput, error) {
		svc, p := reg.liveTV()
		if p != nil {
			return nil, p
		}
		b := in.Body
		lineups, err := svc.LookupXMLSyncLineups(ctx, livetv.XMLSyncLineupsRequest{Country: b.Country, PostalCode: b.PostalCode, Lang: b.Lang})
		if err != nil {
			return nil, liveTVProblem(err)
		}
		return &AdminLiveTVLineupsOutput{Body: NewCollection(mapSlice(lineups, func(l gracenote.LineupOption) LiveTVLineup { return LiveTVLineup(l) }))}, nil
	})
}

func liveTVTunerOf(t livetv.Tuner) LiveTVTuner {
	codecs := t.TranscodeCodecs
	if codecs == nil {
		codecs = []string{}
	}
	return LiveTVTuner{ID: ID(t.ID), Type: t.Type, DeviceID: t.DeviceID, DiscoverURL: t.DiscoverURL, BaseURL: t.BaseURL, Model: t.Model, Firmware: t.Firmware, TunerCount: t.TunerCount, Status: t.Status, ChannelCount: t.ChannelCount, LastError: t.LastError, LastScanAt: liveTVInstantPtr(t.LastScanAt), TranscodeCodecs: codecs}
}

func liveTVGuideSourceOf(s livetv.GuideSource) LiveTVGuideSource {
	config := s.Config
	if config == nil {
		config = map[string]string{}
	}
	return LiveTVGuideSource{ID: ID(s.ID), Type: s.Type, Priority: s.Priority, Enabled: s.Enabled, DisplayName: s.DisplayName, Config: config, Status: s.Status, LastError: s.LastError, LastSyncAt: liveTVInstantPtr(s.LastSyncAt), NextSyncAt: liveTVInstantPtr(s.NextSyncAt)}
}

func liveTVInstantPtr(t *time.Time) *Instant {
	if t == nil {
		return nil
	}
	return liveTVInstant(*t)
}
