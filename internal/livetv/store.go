package livetv

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/idgen"
)

type Store interface {
	ListTuners(ctx context.Context) ([]Tuner, error)
	GetTuner(ctx context.Context, id string) (*Tuner, error)
	CreateTuner(ctx context.Context, tuner *Tuner) (*Tuner, error)
	DeleteTuner(ctx context.Context, id string) error
	ReplaceChannelsForTuner(ctx context.Context, tunerID string, channels []Channel) error
	ListChannels(ctx context.Context, tunerID string) ([]Channel, error)
	GetChannel(ctx context.Context, id string) (*Channel, error)
	UpdateChannel(ctx context.Context, id string, patch ChannelPatch) (*Channel, error)

	ListGuideSources(ctx context.Context, enabledOnly bool) ([]GuideSource, error)
	GetGuideSource(ctx context.Context, id string) (*GuideSource, error)
	CreateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error)
	UpdateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error)
	DeleteGuideSource(ctx context.Context, id string) error
	SetGuideSourceSyncStatus(ctx context.Context, id, status, lastError string, lastSyncAt, nextSyncAt *time.Time) error
	UpsertPrograms(ctx context.Context, sourceID string, programs []Program) error
	ListGuide(ctx context.Context, channelIDs []string, start, end time.Time) ([]Program, error)
	GetProgram(ctx context.Context, id string) (*Program, error)
	ListUpcomingPrograms(ctx context.Context, until time.Time) ([]Program, error)

	ActiveSessionTunerIndices(ctx context.Context, tunerID string) ([]int, error)
	CreateSession(ctx context.Context, input SessionCreate) (*LiveSession, error)
	GetSession(ctx context.Context, id string) (*LiveSession, error)
	ReleaseSession(ctx context.Context, id string) (*LiveSession, error)
	// TouchSession marks an active session as still being watched. id matches
	// either the live session id or its playback bridge session id, so both the
	// MPEG-TS proxy and the live-HLS handler can keep a tuner claimed.
	TouchSession(ctx context.Context, id string) error
	// ReleaseSessionsLastSeenBefore releases active sessions that stopped being
	// watched, returning the rows it reclaimed so their bridges can be stopped.
	ReleaseSessionsLastSeenBefore(ctx context.Context, cutoff time.Time) ([]LiveSession, error)

	ListRecordings(ctx context.Context, status string) ([]Recording, error)
	GetRecording(ctx context.Context, id string) (*Recording, error)
	CreateRecording(ctx context.Context, rec *Recording) (*Recording, error)
	UpdateRecording(ctx context.Context, rec *Recording) (*Recording, error)
	CancelRecording(ctx context.Context, id string) (*Recording, error)
	RecordingExists(ctx context.Context, programID, seriesRuleID string) (bool, error)
	// ListActiveRecordingPairs returns keys of (program_id, series_rule_id) for
	// non-cancelled recordings, for bulk series-rule matching without N×M lookups.
	ListActiveRecordingPairs(ctx context.Context) (map[string]struct{}, error)
	FailDueRecordings(ctx context.Context, now time.Time, message string) (int, error)

	ListSeriesRules(ctx context.Context) ([]SeriesRule, error)
	GetSeriesRule(ctx context.Context, id string) (*SeriesRule, error)
	CreateSeriesRule(ctx context.Context, rule *SeriesRule) (*SeriesRule, error)
	DeleteSeriesRule(ctx context.Context, id string) error
}

type PgStore struct {
	db *pgxpool.Pool
}

func NewPgStore(db *pgxpool.Pool) *PgStore {
	return &PgStore{db: db}
}

func (s *PgStore) ListTuners(ctx context.Context) ([]Tuner, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.type, t.device_id, t.discover_url, t.base_url, t.model, t.firmware,
			t.tuner_count, t.status, COALESCE(c.channel_count, 0), t.last_error, t.last_scan_at
		FROM livetv_tuners t
		LEFT JOIN (
			SELECT tuner_id, COUNT(*) AS channel_count FROM livetv_channels GROUP BY tuner_id
		) c ON c.tuner_id = t.id
		ORDER BY t.created_at`)
	if err != nil {
		return nil, fmt.Errorf("list tuners: %w", err)
	}
	defer rows.Close()
	var tuners []Tuner
	for rows.Next() {
		tuner, err := scanTuner(rows)
		if err != nil {
			return nil, err
		}
		tuners = append(tuners, tuner)
	}
	return tuners, rows.Err()
}

func (s *PgStore) GetTuner(ctx context.Context, id string) (*Tuner, error) {
	row := s.db.QueryRow(ctx, `
		SELECT t.id, t.type, t.device_id, t.discover_url, t.base_url, t.model, t.firmware,
			t.tuner_count, t.status, COALESCE(c.channel_count, 0), t.last_error, t.last_scan_at
		FROM livetv_tuners t
		LEFT JOIN (
			SELECT tuner_id, COUNT(*) AS channel_count FROM livetv_channels GROUP BY tuner_id
		) c ON c.tuner_id = t.id
		WHERE t.id = $1`, id)
	tuner, err := scanTuner(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tuner, nil
}

func (s *PgStore) CreateTuner(ctx context.Context, tuner *Tuner) (*Tuner, error) {
	if tuner.ID == "" {
		id, err := idgen.NextID()
		if err != nil {
			return nil, err
		}
		tuner.ID = id
	}
	if tuner.Type == "" {
		tuner.Type = TunerTypeHDHomeRun
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO livetv_tuners (id, type, device_id, discover_url, base_url, model, firmware, tuner_count, status, last_error, last_scan_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (type, device_id) DO UPDATE SET
			discover_url = EXCLUDED.discover_url,
			base_url = EXCLUDED.base_url,
			model = EXCLUDED.model,
			firmware = EXCLUDED.firmware,
			tuner_count = EXCLUDED.tuner_count,
			status = EXCLUDED.status,
			last_error = EXCLUDED.last_error,
			last_scan_at = COALESCE(EXCLUDED.last_scan_at, livetv_tuners.last_scan_at),
			updated_at = now()
		RETURNING id, type, device_id, discover_url, base_url, model, firmware, tuner_count, status, 0, last_error, last_scan_at`,
		tuner.ID, tuner.Type, tuner.DeviceID, tuner.DiscoverURL, tuner.BaseURL, tuner.Model, tuner.Firmware,
		tuner.TunerCount, tuner.Status, tuner.LastError, tuner.LastScanAt)
	out, err := scanTuner(row)
	if err != nil {
		return nil, fmt.Errorf("create tuner: %w", err)
	}
	return &out, nil
}

func (s *PgStore) DeleteTuner(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM livetv_tuners WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tuner: %w", err)
	}
	return nil
}

func (s *PgStore) ReplaceChannelsForTuner(ctx context.Context, tunerID string, channels []Channel) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace channels: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	existingRows, err := tx.Query(ctx, `SELECT id, number, callsign, stream_url, number_override, enabled, guide_station_id, name, logo_url, hd FROM livetv_channels WHERE tuner_id = $1`, tunerID)
	if err != nil {
		return fmt.Errorf("replace channels: load existing: %w", err)
	}
	existing := map[string]Channel{}
	for existingRows.Next() {
		var ch Channel
		var override sql.NullString
		if err := existingRows.Scan(&ch.ID, &ch.Number, &ch.Callsign, &ch.StreamURL, &override, &ch.Enabled, &ch.GuideStationID, &ch.Name, &ch.LogoURL, &ch.HD); err != nil {
			existingRows.Close()
			return fmt.Errorf("replace channels: scan existing: %w", err)
		}
		ch.NumberOverride = nullStringPtr(override)
		existing[channelKey(ch.Number, ch.Callsign, ch.StreamURL)] = ch
	}
	existingRows.Close()
	if err := existingRows.Err(); err != nil {
		return err
	}

	// Upsert matched channels in place so programs/sessions/recordings that
	// reference kept IDs are not CASCADE-deleted by a wipe-and-reinsert.
	kept := map[string]struct{}{}
	for i := range channels {
		ch := channels[i]
		key := channelKey(ch.Number, ch.Callsign, ch.StreamURL)
		if prev, ok := existing[key]; ok {
			ch.ID = prev.ID
			ch.NumberOverride = prev.NumberOverride
			ch.Enabled = prev.Enabled
			ch.GuideStationID = prev.GuideStationID
			// HDHomeRun lineup has no logos; keep guide-sourced logos across rescans.
			if strings.TrimSpace(ch.LogoURL) == "" {
				ch.LogoURL = prev.LogoURL
			}
			_, err := tx.Exec(ctx, `
				UPDATE livetv_channels SET
					number = $2, callsign = $3, name = $4, logo_url = $5, hd = $6,
					stream_url = $7, sort_key = $8, updated_at = now()
				WHERE id = $1`,
				ch.ID, ch.Number, ch.Callsign, ch.Name, ch.LogoURL, ch.HD, ch.StreamURL, sortKey(ch.Number, i))
			if err != nil {
				return fmt.Errorf("replace channels: update: %w", err)
			}
			kept[key] = struct{}{}
			continue
		}
		if ch.ID == "" {
			id, err := idgen.NextID()
			if err != nil {
				return err
			}
			ch.ID = id
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO livetv_channels (id, tuner_id, number, number_override, callsign, name, logo_url, hd, enabled, stream_url, guide_station_id, sort_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			ch.ID, tunerID, ch.Number, ch.NumberOverride, ch.Callsign, ch.Name, ch.LogoURL, ch.HD, ch.Enabled, ch.StreamURL, ch.GuideStationID, sortKey(ch.Number, i))
		if err != nil {
			return fmt.Errorf("replace channels: insert: %w", err)
		}
	}
	for key, prev := range existing {
		if _, ok := kept[key]; ok {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM livetv_channels WHERE id = $1`, prev.ID); err != nil {
			return fmt.Errorf("replace channels: delete removed: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE livetv_tuners SET status = 'ready', last_error = '', last_scan_at = now(), updated_at = now() WHERE id = $1`, tunerID); err != nil {
		return fmt.Errorf("replace channels: update tuner: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *PgStore) ListChannels(ctx context.Context, tunerID string) ([]Channel, error) {
	sqlText := `SELECT id, tuner_id, number, number_override, callsign, name, logo_url, hd, enabled, stream_url, guide_station_id FROM livetv_channels`
	args := []any{}
	if tunerID != "" {
		sqlText += ` WHERE tuner_id = $1`
		args = append(args, tunerID)
	}
	sqlText += ` ORDER BY sort_key, number, callsign`
	rows, err := s.db.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var channels []Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (s *PgStore) GetChannel(ctx context.Context, id string) (*Channel, error) {
	ch, err := scanChannel(s.db.QueryRow(ctx, `SELECT id, tuner_id, number, number_override, callsign, name, logo_url, hd, enabled, stream_url, guide_station_id FROM livetv_channels WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *PgStore) UpdateChannel(ctx context.Context, id string, patch ChannelPatch) (*Channel, error) {
	row := s.db.QueryRow(ctx, `
		UPDATE livetv_channels SET
			enabled = COALESCE($2, enabled),
			number_override = CASE WHEN $3::boolean THEN $4 ELSE number_override END,
			guide_station_id = CASE WHEN $5::boolean THEN $6 ELSE guide_station_id END,
			logo_url = CASE WHEN $7::boolean THEN $8 ELSE logo_url END,
			updated_at = now()
		WHERE id = $1
		RETURNING id, tuner_id, number, number_override, callsign, name, logo_url, hd, enabled, stream_url, guide_station_id`,
		id, patch.Enabled, patch.NumberOverride != nil, valueOrEmpty(patch.NumberOverride),
		patch.GuideStationID != nil, valueOrEmpty(patch.GuideStationID),
		patch.LogoURL != nil, valueOrEmpty(patch.LogoURL))
	ch, err := scanChannel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return &ch, nil
}

func (s *PgStore) ListGuideSources(ctx context.Context, enabledOnly bool) ([]GuideSource, error) {
	sqlText := `SELECT id, type, priority, enabled, display_name, config_json, status, last_error, last_sync_at, next_sync_at FROM livetv_guide_sources`
	if enabledOnly {
		sqlText += ` WHERE enabled = true`
	}
	sqlText += ` ORDER BY priority, created_at`
	rows, err := s.db.Query(ctx, sqlText)
	if err != nil {
		return nil, fmt.Errorf("list guide sources: %w", err)
	}
	defer rows.Close()
	var sources []GuideSource
	for rows.Next() {
		source, err := scanGuideSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *PgStore) GetGuideSource(ctx context.Context, id string) (*GuideSource, error) {
	source, err := scanGuideSource(s.db.QueryRow(ctx, `SELECT id, type, priority, enabled, display_name, config_json, status, last_error, last_sync_at, next_sync_at FROM livetv_guide_sources WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &source, nil
}

func (s *PgStore) CreateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error) {
	if source.ID == "" {
		id, err := idgen.NextID()
		if err != nil {
			return nil, err
		}
		source.ID = id
	}
	cfg, err := json.Marshal(source.Config)
	if err != nil {
		return nil, fmt.Errorf("guide source config: %w", err)
	}
	if source.Status == "" {
		source.Status = "idle"
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO livetv_guide_sources (id, type, priority, enabled, display_name, config_json, status, last_error, last_sync_at, next_sync_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, type, priority, enabled, display_name, config_json, status, last_error, last_sync_at, next_sync_at`,
		source.ID, source.Type, source.Priority, source.Enabled, source.DisplayName, cfg, source.Status, source.LastError, source.LastSyncAt, source.NextSyncAt)
	out, err := scanGuideSource(row)
	if err != nil {
		return nil, fmt.Errorf("create guide source: %w", err)
	}
	return &out, nil
}

func (s *PgStore) UpdateGuideSource(ctx context.Context, source *GuideSource) (*GuideSource, error) {
	cfg, err := json.Marshal(source.Config)
	if err != nil {
		return nil, fmt.Errorf("guide source config: %w", err)
	}
	row := s.db.QueryRow(ctx, `
		UPDATE livetv_guide_sources SET
			type = $2, priority = $3, enabled = $4, display_name = $5, config_json = $6,
			status = $7, last_error = $8, next_sync_at = $9, updated_at = now()
		WHERE id = $1
		RETURNING id, type, priority, enabled, display_name, config_json, status, last_error, last_sync_at, next_sync_at`,
		source.ID, source.Type, source.Priority, source.Enabled, source.DisplayName, cfg, source.Status, source.LastError, source.NextSyncAt)
	out, err := scanGuideSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update guide source: %w", err)
	}
	return &out, nil
}

func (s *PgStore) DeleteGuideSource(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM livetv_guide_sources WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete guide source: %w", err)
	}
	return nil
}

func (s *PgStore) SetGuideSourceSyncStatus(ctx context.Context, id, status, lastError string, lastSyncAt, nextSyncAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE livetv_guide_sources
		SET status = $2, last_error = $3, last_sync_at = COALESCE($4, last_sync_at), next_sync_at = $5, updated_at = now()
		WHERE id = $1`, id, status, lastError, lastSyncAt, nextSyncAt)
	if err != nil {
		return fmt.Errorf("set guide source sync status: %w", err)
	}
	return nil
}

func (s *PgStore) UpsertPrograms(ctx context.Context, sourceID string, programs []Program) error {
	if len(programs) == 0 {
		return nil
	}
	start, end := programs[0].Start, programs[0].Stop
	channelSet := map[string]struct{}{}
	for i := range programs {
		if programs[i].Start.Before(start) {
			start = programs[i].Start
		}
		if programs[i].Stop.After(end) {
			end = programs[i].Stop
		}
		channelSet[programs[i].ChannelID] = struct{}{}
	}
	channelIDs := make([]string, 0, len(channelSet))
	for id := range channelSet {
		channelIDs = append(channelIDs, id)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("upsert programs: begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM livetv_programs WHERE source_id = $1 AND channel_id = ANY($2) AND start_at < $3 AND stop_at > $4`, sourceID, channelIDs, end, start); err != nil {
		return fmt.Errorf("upsert programs: clear window: %w", err)
	}
	for i := range programs {
		p := programs[i]
		if p.ID == "" {
			id, err := idgen.NextID()
			if err != nil {
				return err
			}
			p.ID = id
		}
		if p.SourceID == "" {
			p.SourceID = sourceID
		}
		genres := nonNilStringSlice(p.Genres)
		_, err := tx.Exec(ctx, `
			INSERT INTO livetv_programs (id, channel_id, source_id, series_id, external_id, start_at, stop_at, title, subtitle, description, season, episode, genres, image_url, is_new, is_live)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			p.ID, p.ChannelID, p.SourceID, p.SeriesID, p.ExternalID, p.Start, p.Stop, p.Title, p.Subtitle,
			p.Description, p.Season, p.Episode, genres, p.ImageURL, p.IsNew, p.IsLive)
		if err != nil {
			return fmt.Errorf("upsert programs: insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *PgStore) ListGuide(ctx context.Context, channelIDs []string, start, end time.Time) ([]Program, error) {
	sqlText := `SELECT id, channel_id, COALESCE(source_id, ''), series_id, external_id, start_at, stop_at, title, subtitle, description, season, episode, genres, image_url, is_new, is_live FROM livetv_programs WHERE start_at < $1 AND stop_at > $2`
	args := []any{end, start}
	if len(channelIDs) > 0 {
		sqlText += ` AND channel_id = ANY($3)`
		args = append(args, channelIDs)
	}
	sqlText += ` ORDER BY start_at, channel_id`
	return s.listPrograms(ctx, sqlText, args...)
}

func (s *PgStore) GetProgram(ctx context.Context, id string) (*Program, error) {
	program, err := scanProgram(s.db.QueryRow(ctx, `SELECT id, channel_id, COALESCE(source_id, ''), series_id, external_id, start_at, stop_at, title, subtitle, description, season, episode, genres, image_url, is_new, is_live FROM livetv_programs WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &program, nil
}

func (s *PgStore) ListUpcomingPrograms(ctx context.Context, until time.Time) ([]Program, error) {
	return s.listPrograms(ctx, `SELECT id, channel_id, COALESCE(source_id, ''), series_id, external_id, start_at, stop_at, title, subtitle, description, season, episode, genres, image_url, is_new, is_live FROM livetv_programs WHERE stop_at > now() AND start_at <= $1 ORDER BY start_at`, until)
}

func (s *PgStore) listPrograms(ctx context.Context, sqlText string, args ...any) ([]Program, error) {
	rows, err := s.db.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list programs: %w", err)
	}
	defer rows.Close()
	var programs []Program
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, err
		}
		programs = append(programs, p)
	}
	return programs, rows.Err()
}

func (s *PgStore) ActiveSessionTunerIndices(ctx context.Context, tunerID string) ([]int, error) {
	rows, err := s.db.Query(ctx, `SELECT tuner_index FROM livetv_sessions WHERE tuner_id = $1 AND status = 'active'`, tunerID)
	if err != nil {
		return nil, fmt.Errorf("active session tuner indices: %w", err)
	}
	defer rows.Close()
	var indices []int
	for rows.Next() {
		var index int
		if err := rows.Scan(&index); err != nil {
			return nil, err
		}
		indices = append(indices, index)
	}
	return indices, rows.Err()
}

const sessionSelectCols = `id, channel_id, tuner_id, tuner_index, user_id, profile_id, playback_session_id, status, created_at, released_at, last_seen_at`

func (s *PgStore) CreateSession(ctx context.Context, input SessionCreate) (*LiveSession, error) {
	id, err := idgen.NextID()
	if err != nil {
		return nil, err
	}
	session, err := scanSession(s.db.QueryRow(ctx, `
		INSERT INTO livetv_sessions (id, channel_id, tuner_id, tuner_index, user_id, profile_id, playback_session_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')
		RETURNING `+sessionSelectCols,
		id, input.ChannelID, input.TunerID, input.TunerIndex, nullInt(input.UserID), input.ProfileID, input.PlaybackSessionID))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrTunerIndexConflict
		}
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session, nil
}

func (s *PgStore) GetSession(ctx context.Context, id string) (*LiveSession, error) {
	session, err := scanSession(s.db.QueryRow(ctx, `SELECT `+sessionSelectCols+` FROM livetv_sessions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *PgStore) ReleaseSession(ctx context.Context, id string) (*LiveSession, error) {
	session, err := scanSession(s.db.QueryRow(ctx, `
		UPDATE livetv_sessions SET status = 'released', released_at = now()
		WHERE id = $1
		RETURNING `+sessionSelectCols, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("release session: %w", err)
	}
	return &session, nil
}

func (s *PgStore) TouchSession(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE livetv_sessions SET last_seen_at = now()
		WHERE status = 'active' AND (id = $1 OR playback_session_id = $1)`, id); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *PgStore) ReleaseSessionsLastSeenBefore(ctx context.Context, cutoff time.Time) ([]LiveSession, error) {
	rows, err := s.db.Query(ctx, `
		UPDATE livetv_sessions SET status = 'released', released_at = now()
		WHERE status = 'active' AND last_seen_at < $1
		RETURNING `+sessionSelectCols, cutoff)
	if err != nil {
		return nil, fmt.Errorf("release stale sessions: %w", err)
	}
	defer rows.Close()
	var released []LiveSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		released = append(released, session)
	}
	return released, rows.Err()
}

const recordingSelectCols = `id, COALESCE(program_id, ''), channel_id, COALESCE(series_rule_id, ''), user_id, COALESCE(profile_id, ''), status, path, COALESCE(library_item_id, ''), start_at, stop_at, title, last_error`

func (s *PgStore) ListRecordings(ctx context.Context, status string) ([]Recording, error) {
	sqlText := `SELECT ` + recordingSelectCols + ` FROM livetv_recordings`
	args := []any{}
	if status != "" {
		sqlText += ` WHERE status = $1`
		args = append(args, status)
	}
	sqlText += ` ORDER BY start_at DESC`
	rows, err := s.db.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	defer rows.Close()
	var recordings []Recording
	for rows.Next() {
		rec, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		recordings = append(recordings, rec)
	}
	return recordings, rows.Err()
}

func (s *PgStore) GetRecording(ctx context.Context, id string) (*Recording, error) {
	rec, err := scanRecording(s.db.QueryRow(ctx, `SELECT `+recordingSelectCols+` FROM livetv_recordings WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recording: %w", err)
	}
	return &rec, nil
}

func (s *PgStore) CreateRecording(ctx context.Context, rec *Recording) (*Recording, error) {
	if rec.ID == "" {
		id, err := idgen.NextID()
		if err != nil {
			return nil, err
		}
		rec.ID = id
	}
	if rec.Status == "" {
		rec.Status = "scheduled"
	}
	out, err := scanRecording(s.db.QueryRow(ctx, `
		INSERT INTO livetv_recordings (id, program_id, channel_id, series_rule_id, user_id, profile_id, status, path, library_item_id, start_at, stop_at, title, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+recordingSelectCols,
		rec.ID, nullString(rec.ProgramID), rec.ChannelID, nullString(rec.SeriesRuleID), nullInt(rec.UserID), rec.ProfileID, rec.Status, rec.Path, nullString(rec.LibraryItemID), rec.Start, rec.Stop, rec.Title, rec.LastError))
	if err != nil {
		return nil, fmt.Errorf("create recording: %w", err)
	}
	return &out, nil
}

func (s *PgStore) UpdateRecording(ctx context.Context, rec *Recording) (*Recording, error) {
	if rec == nil || rec.ID == "" {
		return nil, fmt.Errorf("update recording: id required")
	}
	out, err := scanRecording(s.db.QueryRow(ctx, `
		UPDATE livetv_recordings
		SET status = $2,
			path = $3,
			library_item_id = $4,
			last_error = $5,
			start_at = $6,
			stop_at = $7,
			title = $8,
			updated_at = now()
		WHERE id = $1
		RETURNING `+recordingSelectCols,
		rec.ID, rec.Status, rec.Path, nullString(rec.LibraryItemID), rec.LastError, rec.Start, rec.Stop, rec.Title))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update recording: %w", err)
	}
	return &out, nil
}

func (s *PgStore) CancelRecording(ctx context.Context, id string) (*Recording, error) {
	rec, err := scanRecording(s.db.QueryRow(ctx, `
		UPDATE livetv_recordings SET status = 'cancelled', updated_at = now()
		WHERE id = $1
		RETURNING `+recordingSelectCols, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cancel recording: %w", err)
	}
	return &rec, nil
}

func (s *PgStore) RecordingExists(ctx context.Context, programID, seriesRuleID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM livetv_recordings WHERE program_id IS NOT DISTINCT FROM $1 AND series_rule_id IS NOT DISTINCT FROM $2 AND status <> 'cancelled')`, nullString(programID), nullString(seriesRuleID)).Scan(&exists)
	return exists, err
}

func (s *PgStore) ListActiveRecordingPairs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.Query(ctx, `
		SELECT COALESCE(program_id, ''), COALESCE(series_rule_id, '')
		FROM livetv_recordings
		WHERE status <> 'cancelled'`)
	if err != nil {
		return nil, fmt.Errorf("list active recording pairs: %w", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var programID, seriesRuleID string
		if err := rows.Scan(&programID, &seriesRuleID); err != nil {
			return nil, err
		}
		out[recordingPairKey(programID, seriesRuleID)] = struct{}{}
	}
	return out, rows.Err()
}

func (s *PgStore) FailDueRecordings(ctx context.Context, now time.Time, message string) (int, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE livetv_recordings
		SET status = 'failed', last_error = $2, updated_at = now()
		WHERE status = 'scheduled' AND start_at <= $1`, now, message)
	if err != nil {
		return 0, fmt.Errorf("fail due recordings: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

const seriesRuleSelectCols = `id, series_id, channel_id, user_id, COALESCE(profile_id, ''), title_match, new_only, keep_last, enabled`

func (s *PgStore) ListSeriesRules(ctx context.Context) ([]SeriesRule, error) {
	rows, err := s.db.Query(ctx, `SELECT `+seriesRuleSelectCols+` FROM livetv_series_rules ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list series rules: %w", err)
	}
	defer rows.Close()
	var rules []SeriesRule
	for rows.Next() {
		rule, err := scanSeriesRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *PgStore) GetSeriesRule(ctx context.Context, id string) (*SeriesRule, error) {
	rule, err := scanSeriesRule(s.db.QueryRow(ctx, `SELECT `+seriesRuleSelectCols+` FROM livetv_series_rules WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get series rule: %w", err)
	}
	return &rule, nil
}

func (s *PgStore) CreateSeriesRule(ctx context.Context, rule *SeriesRule) (*SeriesRule, error) {
	if rule.ID == "" {
		id, err := idgen.NextID()
		if err != nil {
			return nil, err
		}
		rule.ID = id
	}
	out, err := scanSeriesRule(s.db.QueryRow(ctx, `
		INSERT INTO livetv_series_rules (id, series_id, channel_id, user_id, profile_id, title_match, new_only, keep_last, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+seriesRuleSelectCols,
		rule.ID, rule.SeriesID, rule.ChannelID, nullInt(rule.UserID), rule.ProfileID, rule.TitleMatch, rule.NewOnly, rule.KeepLast, rule.Enabled))
	if err != nil {
		return nil, fmt.Errorf("create series rule: %w", err)
	}
	return &out, nil
}

func (s *PgStore) DeleteSeriesRule(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM livetv_series_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete series rule: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTuner(row scanner) (Tuner, error) {
	var tuner Tuner
	var lastScan sql.NullTime
	if err := row.Scan(&tuner.ID, &tuner.Type, &tuner.DeviceID, &tuner.DiscoverURL, &tuner.BaseURL, &tuner.Model,
		&tuner.Firmware, &tuner.TunerCount, &tuner.Status, &tuner.ChannelCount, &tuner.LastError, &lastScan); err != nil {
		return Tuner{}, err
	}
	tuner.LastScanAt = nullTimePtr(lastScan)
	return tuner, nil
}

func scanChannel(row scanner) (Channel, error) {
	var ch Channel
	var override sql.NullString
	if err := row.Scan(&ch.ID, &ch.TunerID, &ch.Number, &override, &ch.Callsign, &ch.Name, &ch.LogoURL, &ch.HD, &ch.Enabled, &ch.StreamURL, &ch.GuideStationID); err != nil {
		return Channel{}, err
	}
	ch.NumberOverride = nullStringPtr(override)
	return ch, nil
}

func scanGuideSource(row scanner) (GuideSource, error) {
	var source GuideSource
	var cfgBytes []byte
	var lastSync, nextSync sql.NullTime
	if err := row.Scan(&source.ID, &source.Type, &source.Priority, &source.Enabled, &source.DisplayName, &cfgBytes, &source.Status, &source.LastError, &lastSync, &nextSync); err != nil {
		return GuideSource{}, err
	}
	if len(cfgBytes) > 0 {
		_ = json.Unmarshal(cfgBytes, &source.Config)
	}
	if source.Config == nil {
		source.Config = map[string]string{}
	}
	source.LastSyncAt = nullTimePtr(lastSync)
	source.NextSyncAt = nullTimePtr(nextSync)
	return source, nil
}

func scanProgram(row scanner) (Program, error) {
	var p Program
	var season, episode sql.NullInt64
	if err := row.Scan(&p.ID, &p.ChannelID, &p.SourceID, &p.SeriesID, &p.ExternalID, &p.Start, &p.Stop, &p.Title, &p.Subtitle,
		&p.Description, &season, &episode, &p.Genres, &p.ImageURL, &p.IsNew, &p.IsLive); err != nil {
		return Program{}, err
	}
	if season.Valid {
		n := int(season.Int64)
		p.Season = &n
	}
	if episode.Valid {
		n := int(episode.Int64)
		p.Episode = &n
	}
	return p, nil
}

func scanSession(row scanner) (LiveSession, error) {
	var session LiveSession
	var userID sql.NullInt64
	var released sql.NullTime
	if err := row.Scan(&session.ID, &session.ChannelID, &session.TunerID, &session.TunerIndex, &userID, &session.ProfileID,
		&session.PlaybackSessionID, &session.Status, &session.CreatedAt, &released, &session.LastSeenAt); err != nil {
		return LiveSession{}, err
	}
	if userID.Valid {
		session.UserID = int(userID.Int64)
	}
	session.ReleasedAt = nullTimePtr(released)
	return session, nil
}

func scanRecording(row scanner) (Recording, error) {
	var rec Recording
	var userID sql.NullInt64
	if err := row.Scan(&rec.ID, &rec.ProgramID, &rec.ChannelID, &rec.SeriesRuleID, &userID, &rec.ProfileID, &rec.Status, &rec.Path, &rec.LibraryItemID,
		&rec.Start, &rec.Stop, &rec.Title, &rec.LastError); err != nil {
		return Recording{}, err
	}
	if userID.Valid {
		rec.UserID = int(userID.Int64)
	}
	return rec, nil
}

func scanSeriesRule(row scanner) (SeriesRule, error) {
	var rule SeriesRule
	var channelID sql.NullString
	var userID sql.NullInt64
	if err := row.Scan(&rule.ID, &rule.SeriesID, &channelID, &userID, &rule.ProfileID, &rule.TitleMatch, &rule.NewOnly, &rule.KeepLast, &rule.Enabled); err != nil {
		return SeriesRule{}, err
	}
	rule.ChannelID = nullStringPtr(channelID)
	if userID.Valid {
		rule.UserID = int(userID.Int64)
	}
	return rule, nil
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// nonNilStringSlice maps nil to an empty slice so NOT NULL text[] columns
// (and JSON responses) never see a SQL/JSON null.
func nonNilStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func channelKey(number, callsign, streamURL string) string {
	return strings.ToLower(strings.TrimSpace(number) + "|" + strings.TrimSpace(callsign) + "|" + strings.TrimSpace(streamURL))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func sortKey(number string, fallback int) int {
	parts := strings.FieldsFunc(number, func(r rune) bool { return r == '.' || r == '-' })
	if len(parts) == 0 {
		return math.MaxInt32 - fallback
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return math.MaxInt32 - fallback
	}
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major*1000 + minor
}
