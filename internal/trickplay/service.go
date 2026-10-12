package trickplay

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prairie-server/prairie-server/internal/config"
	"github.com/prairie-server/prairie-server/internal/database/pglock"
	"github.com/prairie-server/prairie-server/internal/imageutil"
	"github.com/prairie-server/prairie-server/internal/mediasample"
	"github.com/prairie-server/prairie-server/internal/models"
	"github.com/prairie-server/prairie-server/internal/tonemap"
)

// Settings the service reads live, so a change applies to the next claim.
const (
	// WidthSetting is shared with chapter thumbnails: one preview image
	// width for both.
	WidthSetting     = config.PreviewImageWidthSettingKey
	IntervalSetting  = "playback.trickplay_interval_seconds"
	WorkersSetting   = "playback.trickplay_workers"
	ExecutionSetting = "playback.trickplay_execution"

	ffmpegPathSetting = "playback.ffmpeg_path"
	hwAccelSetting    = "playback.hw_accel"
	hwDeviceSetting   = "playback.hw_device"
)

// Execution modes of ExecutionSetting.
const (
	ExecutionLocal                = "local"
	ExecutionPreferTranscodeNodes = "prefer_transcode_nodes"
	ExecutionTranscodeNodesOnly   = "transcode_nodes_only"
)

const (
	// leaseDuration is how long a claim holds a file without a heartbeat;
	// heartbeatInterval renews it well before then.
	leaseDuration     = 90 * time.Second
	heartbeatInterval = 30 * time.Second
	// idlePoll is how often an idle server looks for work it was not told
	// about; Kick wakes it sooner.
	idlePoll = time.Minute
	// reconcileRetryDelay spaces another attempt at a requested reconcile
	// that another server's lock or an error skipped.
	reconcileRetryDelay = 30 * time.Second
	// maxChunkSheets bounds the sheets one ffmpeg run makes, which bounds
	// the memory of a run and the size of a transcode node's answer.
	maxChunkSheets = 16
	// Bound decoded output as well as sheet count. The first sheet learns
	// actual display geometry before batching subsequent sheets.
	maxChunkPixels = 64 << 20
	// runThreads caps each ffmpeg run's decoder and filter threads. Work
	// runs at idle priority, so more parallelism comes from more workers.
	runThreads = 2
	// reconcileBatch bounds each reconcile step.
	reconcileBatch = 5000
	// reconcilePassBatches yields the cluster lock after a bounded pass;
	// a full final batch schedules another pass immediately.
	reconcilePassBatches = 4
	// releaseDelay is how long a file waits after this server gave it back
	// through no fault of the file: its input unreadable here, or shutdown.
	releaseDelay = time.Hour
)

// reconcileLock serializes reconcile passes across servers ("PRAIRIETRKP").
const reconcileLock int64 = 0x53494C4F54524B50

// errLeaseLost ends a generation whose lease another server now holds.
var errLeaseLost = errors.New("trickplay lease lost")

// SettingsReader reads admin settings.
type SettingsReader interface {
	Get(ctx context.Context, key string) (string, error)
}

// Store is where sheets are written.
type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Identity() string
}

// Extractor makes one sheets request's sheets for a job: on this server or
// on a transcode node.
type Extractor interface {
	Extract(ctx context.Context, job *Job, req mediasample.Request) (mediasample.Result, error)
}

// queue is the Repository surface the service uses; tests replace it.
type queue interface {
	Claim(ctx context.Context, owner string, lease time.Duration) (*Job, error)
	Heartbeat(ctx context.Context, fileID int, owner string, lease time.Duration) (bool, error)
	BeginUpload(ctx context.Context, fileID int, owner string) (int64, bool, error)
	Publish(ctx context.Context, fileID int, owner string, revision int64, p Published) (bool, error)
	Finish(ctx context.Context, fileID int, owner string, outcome Outcome, cause string, delay time.Duration) (bool, error)
	Reconcile(ctx context.Context, recipe Recipe, storeIdentity string, batch int) (ReconcileStats, error)
}

// Service generates the sheets of queued files. Every server runs one; the
// number of files a server works on at once follows WorkersSetting.
type Service struct {
	pool      *pgxpool.Pool
	queue     queue
	store     Store
	settings  SettingsReader
	extractor Extractor
	owner     string
	limiter   *mediasample.Limiter
	logger    *slog.Logger
	wake      chan struct{}
	reconcile chan struct{}
	now       func() time.Time
	// heartbeatEvery paces lease renewals, settingsEvery paces rereading
	// the settings, and reconcileRetry paces another attempt at a requested
	// pass that another server's lock or an error skipped; tests replace
	// them, and reconcilePass.
	heartbeatEvery time.Duration
	settingsEvery  time.Duration
	reconcileRetry time.Duration
	reconcilePass  func(ctx context.Context) (ReconcileStats, bool, error)
}

// NewService returns a service that claims work as nodeID. It returns nil
// without a database or a store.
func NewService(pool *pgxpool.Pool, store Store, settings SettingsReader, extractor Extractor, nodeID string) *Service {
	if pool == nil || store == nil || extractor == nil {
		return nil
	}
	s := newService(NewRepository(pool), store, settings, extractor, nodeID)
	s.pool = pool
	return s
}

func newService(q queue, store Store, settings SettingsReader, extractor Extractor, nodeID string) *Service {
	s := &Service{
		queue:          q,
		store:          store,
		settings:       settings,
		extractor:      extractor,
		owner:          leaseOwner(nodeID),
		limiter:        mediasample.NewLimiter(1),
		logger:         slog.Default().With("component", "trickplay"),
		wake:           make(chan struct{}, 1),
		reconcile:      make(chan struct{}, 1),
		now:            time.Now,
		heartbeatEvery: heartbeatInterval,
		settingsEvery:  idlePoll,
		reconcileRetry: reconcileRetryDelay,
	}
	s.reconcilePass = s.Reconcile
	return s
}

// leaseOwner names this process in leases: the node, and a token that is
// new on every start, so a restarted server never acts on its previous
// process's leases.
func leaseOwner(nodeID string) string {
	var token [4]byte
	_, _ = rand.Read(token[:])
	return strings.TrimSpace(nodeID) + ":" + hex.EncodeToString(token[:])
}

// Kick wakes the service to look for work now.
func (s *Service) Kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// ReconcileSoon runs a reconcile pass in the background, as after a
// settings change. Requests made while one runs fold into the next.
func (s *Service) ReconcileSoon() {
	select {
	case s.reconcile <- struct{}{}:
	default:
	}
}

// Start runs the service until ctx ends.
func (s *Service) Start(ctx context.Context) {
	go s.dispatch(ctx)
	go s.reconcileOnRequest(ctx)
}

func (s *Service) reconcileOnRequest(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reconcile:
		}
		_, ran, err := s.reconcilePass(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.WarnContext(ctx, "trickplay reconcile failed", "error", err)
		}
		if ran && err == nil {
			continue
		}
		// Another server held the lock or the pass failed. A settings change
		// is noticed once per server, so try again rather than leave the
		// old recipe in place until the periodic pass.
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.reconcileRetry):
		}
		s.ReconcileSoon()
	}
}

// dispatch claims files while a worker slot is free, and runs each in its
// own goroutine. The number of slots follows WorkersSetting.
func (s *Service) dispatch(ctx context.Context) {
	var running sync.WaitGroup
	defer running.Wait()
	running.Go(func() { s.followSettings(ctx) })
	for ctx.Err() == nil {
		release, err := s.limiter.Acquire(ctx)
		if err != nil {
			return
		}
		job, err := s.queue.Claim(ctx, s.owner, leaseDuration)
		if err != nil || job == nil {
			release()
			if err != nil && ctx.Err() == nil {
				s.logger.WarnContext(ctx, "claim trickplay work failed", "error", err)
			}
			s.idle(ctx)
			continue
		}
		running.Go(func() {
			defer release()
			s.process(ctx, job)
		})
	}
}

// followSettings applies setting changes as they are read: the worker count
// resizes the limiter, and a new width or interval reconciles right away
// rather than at the next periodic reconcile.
func (s *Service) followSettings(ctx context.Context) {
	var recipe string
	for {
		s.limiter.Resize(s.intSetting(ctx, WorkersSetting, 1, 1, 64))
		if current, err := s.Recipe(ctx); err == nil {
			if recipe != "" && current.String() != recipe {
				s.ReconcileSoon()
			}
			recipe = current.String()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.settingsEvery):
		}
	}
}

// idle waits for a kick or the idle poll.
func (s *Service) idle(ctx context.Context) {
	timer := time.NewTimer(idlePoll)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-s.wake:
	case <-timer.C:
	}
}

// process generates job's sheets and records how it ended.
func (s *Service) process(ctx context.Context, job *Job) {
	started := s.now()
	workCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		s.heartbeat(workCtx, job, cancel)
	}()
	published, err := s.generate(workCtx, job)
	cancel(nil)
	<-heartbeatDone

	// The outcome is recorded even when the service is stopping.
	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer recordCancel()
	log := s.logger.With("file_id", job.FileID, "duration", s.now().Sub(started).Round(time.Millisecond))
	if err == nil {
		log.InfoContext(ctx, "trickplay sheets published", "thumbnails", published.Count, "sheets", len(published.SheetBytes),
			"decoder", published.Decoder, "filled", published.Filled)
		// A reconcile run while this job held its lease skipped the row, so
		// sheets made to settings that changed meanwhile are requeued now
		// rather than at the periodic pass.
		if current, err := s.Recipe(recordCtx); err == nil && current != published.Recipe {
			s.ReconcileSoon()
		}
		return
	}
	outcome, delay := s.classify(ctx, err)
	if outcome < 0 {
		log.InfoContext(ctx, "trickplay generation abandoned", "reason", err)
		return
	}
	if _, finishErr := s.queue.Finish(recordCtx, job.FileID, job.LeaseToken, outcome, err.Error(), delay); finishErr != nil {
		log.WarnContext(ctx, "record trickplay outcome failed", "error", finishErr)
	}
	level := slog.LevelWarn
	if outcome == Released {
		level = slog.LevelInfo
	}
	log.Log(ctx, level, "trickplay generation did not publish", "outcome", outcome, "error", err)
}

// classify maps a generation error to how the row ends; a negative outcome
// leaves the row alone because this server no longer holds it.
func (s *Service) classify(ctx context.Context, err error) (Outcome, time.Duration) {
	_, unreadable := errors.AsType[*inputError](err)
	switch {
	case errors.Is(err, errLeaseLost):
		return -1, 0
	case ctx.Err() != nil:
		// Shutting down: give the file back for another server.
		return Released, 0
	case unreadable:
		return Released, releaseDelay
	case errors.Is(err, errNoNode), errors.Is(err, errSettingsUnreadable):
		return Released, time.Minute
	case mediasample.Classify(err).Permanent():
		return Unusable, 0
	}
	if failure, ok := errors.AsType[*ExtractError](err); ok && failure.Permanent {
		return Unusable, 0
	}
	return Failed, 0
}

// inputError is a job whose file this server cannot read, such as an
// offline mount: no fault of the file, so no failure is counted.
type inputError struct{ err error }

func (e *inputError) Error() string { return "input not readable here: " + e.err.Error() }
func (e *inputError) Unwrap() error { return e.err }

// errNoNode is a job that must run on a transcode node when none is
// available.
var errNoNode = errors.New("no transcode node can make trickplay sheets")

// heartbeat renews the lease until ctx ends, and cancels the work with
// errLeaseLost once the lease is gone: when a renewal is refused, or when
// renewals have failed for as long as the lease lasts.
func (s *Service) heartbeat(ctx context.Context, job *Job, cancel context.CancelCauseFunc) {
	ticker := time.NewTicker(s.heartbeatEvery)
	defer ticker.Stop()
	renewed := s.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		ok, err := s.queue.Heartbeat(ctx, job.FileID, job.LeaseToken, leaseDuration)
		switch {
		case err == nil && !ok:
			cancel(errLeaseLost)
			return
		case err == nil:
			renewed = s.now()
		case s.now().Sub(renewed) >= leaseDuration:
			cancel(errLeaseLost)
			return
		}
	}
}

// generate makes, uploads, and publishes job's sheets.
func (s *Service) generate(ctx context.Context, job *Job) (Published, error) {
	recipe, err := s.Recipe(ctx)
	if err != nil {
		return Published{}, err
	}
	track := primaryVideoTrack(job.VideoTracks)
	height := recipe.TileHeight(displayAspect(track))
	times := recipe.SampleTimes(float64(job.DurationSeconds))
	if len(times) == 0 {
		return Published{}, &mediasample.Error{Reason: mediasample.ReasonNoStream}
	}
	columns, rows := recipe.Grid()
	perSheet := columns * rows
	sheets := &mediasample.SheetsOutput{TileWidth: recipe.Width, TileHeight: height, Columns: columns, Rows: rows, Quality: Quality, UseInputAspect: true}
	if tonemap.NeedsToneMap(&models.MediaFile{HDR: job.HDR, VideoTracks: job.VideoTracks}) {
		sheets.ToneMap = &mediasample.ToneMap{AllowSoftware: true}
	}
	published := Published{Recipe: recipe, StoreIdentity: s.store.Identity(), Height: height, Count: len(times)}
	var revision int64
	chunkSheets := 1
	for start := 0; start < len(times); {
		chunk := times[start:min(len(times), start+perSheet*chunkSheets)]
		req := mediasample.Request{
			Input:         job.FilePath,
			Samples:       &mediasample.Samples{Seconds: chunk, ReadThrough: readThrough(job)},
			Sheets:        sheets,
			Threads:       runThreads,
			VideoBitDepth: mediasample.VideoBitDepthHint(track.BitDepth),
			Background:    true,
		}
		result, err := s.extractor.Extract(ctx, job, req)
		if err != nil {
			return Published{}, leaseAware(ctx, err)
		}
		actualHeight := height
		if result.SheetTileHeight > 0 {
			actualHeight = result.SheetTileHeight
		}
		if start == 0 {
			published.Height = actualHeight
		} else if actualHeight != published.Height {
			return Published{}, fmt.Errorf("trickplay chunks have different tile heights: %d and %d", published.Height, actualHeight)
		}
		chunkSheets = max(1, min(maxChunkSheets, maxChunkPixels/(recipe.Width*actualHeight*perSheet)))
		if revision == 0 {
			rev, ok, err := s.queue.BeginUpload(ctx, job.FileID, job.LeaseToken)
			if err != nil {
				return Published{}, leaseAware(ctx, err)
			}
			if !ok {
				return Published{}, errLeaseLost
			}
			revision = rev
		}
		for _, sheet := range result.Sheets {
			key := SheetKey(job.FileID, revision, start/perSheet+sheet.Index)
			if err := s.store.Put(ctx, key, sheet.JPEG); err != nil {
				return Published{}, leaseAware(ctx, fmt.Errorf("store %s: %w", key, err))
			}
			published.SheetBytes = append(published.SheetBytes, len(sheet.JPEG))
		}
		published.Filled += result.SheetFrames.Filled
		published.Decoder = result.Decoder
		start += len(chunk)
	}
	ok, err := s.queue.Publish(ctx, job.FileID, job.LeaseToken, revision, published)
	if err != nil {
		return Published{}, leaseAware(ctx, err)
	}
	if !ok {
		return Published{}, errLeaseLost
	}
	return published, nil
}

// leaseAware reports a lost lease rather than the error it caused.
func leaseAware(ctx context.Context, err error) error {
	if errors.Is(context.Cause(ctx), errLeaseLost) {
		return errLeaseLost
	}
	return err
}

// readThrough reports whether job's samples are read in one pass instead
// of seeking to each (see mediasample.Samples.ReadThrough). Seeking reads a
// fraction of most files and decodes a keyframe per thumbnail, but AVI
// seeks by its index poorly: on a 0.73 GB AVI fixture, seeking read 3 GB and
// took 46 s where one pass read the file once in 6 s.
func readThrough(job *Job) bool {
	return strings.EqualFold(strings.TrimSpace(job.Container), "avi")
}

// primaryVideoTrack is the first video track, the one mediasample decodes.
func primaryVideoTrack(tracks []models.VideoTrack) models.VideoTrack {
	if len(tracks) > 0 {
		return tracks[0]
	}
	return models.VideoTrack{}
}

// displayAspect is the track's display aspect ratio: the probed one, or its
// pixel dimensions when the probe gave none. Zero means unknown.
func displayAspect(track models.VideoTrack) float64 {
	if aspect, ok := ParseAspectRatio(track.AspectRatio); ok {
		return aspect
	}
	if track.Width > 0 && track.Height > 0 {
		return float64(track.Width) / float64(track.Height)
	}
	return 0
}

// errSettingsUnreadable is a recipe the settings could not be read for.
// Neither a reconcile nor a generation goes on with the default recipe
// instead: every file published to another one would be made again.
var errSettingsUnreadable = errors.New("trickplay settings could not be read")

// Recipe is the recipe the current settings describe. An unset setting takes
// its default; one that cannot be read is errSettingsUnreadable.
func (s *Service) Recipe(ctx context.Context) (Recipe, error) {
	values := map[string]int{WidthSetting: DefaultWidth, IntervalSetting: DefaultIntervalSeconds}
	for key := range values {
		if s.settings == nil {
			break
		}
		raw, err := s.settings.Get(ctx, key)
		if err != nil {
			return Recipe{}, fmt.Errorf("%w: %s: %w", errSettingsUnreadable, key, err)
		}
		if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			values[key] = value
		}
	}
	return NewRecipe(values[WidthSetting], values[IntervalSetting]), nil
}

// Reconcile runs one reconcile pass, unless another server is running one,
// and wakes the workers.
func (s *Service) Reconcile(ctx context.Context) (ReconcileStats, bool, error) {
	// Settings may use the same pool. Read them before reserving its session.
	recipe, err := s.Recipe(ctx)
	if err != nil {
		return ReconcileStats{}, false, err
	}
	lock, acquired, err := pglock.TryAcquire(ctx, s.pool, reconcileLock)
	if err != nil {
		return ReconcileStats{}, false, fmt.Errorf("take the trickplay reconcile lock: %w", err)
	}
	if !acquired {
		return ReconcileStats{}, false, nil
	}
	more := false
	defer func() {
		_ = lock.Release(context.WithoutCancel(ctx))
		if more && ctx.Err() == nil {
			s.ReconcileSoon()
		}
	}()
	reconcile := s.queue.Reconcile
	if repository, ok := s.queue.(*Repository); ok {
		// Keep the SQL on the lock's session, including with a pool of one
		// connection. Losing that session also stops the guarded work.
		reconcile = func(ctx context.Context, recipe Recipe, storeIdentity string, batch int) (ReconcileStats, error) {
			return repository.reconcile(ctx, lock.Conn().Exec, recipe, storeIdentity, batch)
		}
	}
	stats, more, err := s.reconcileBatches(ctx, recipe, reconcile)
	return stats, true, err
}

// reconcileBatches drains bounded steps, waking workers after each batch.
// A pass yields after reconcilePassBatches so other servers and settings
// changes can take the lock before a continuation.
func (s *Service) reconcileBatches(ctx context.Context, recipe Recipe, reconcile func(context.Context, Recipe, string, int) (ReconcileStats, error)) (ReconcileStats, bool, error) {
	var total ReconcileStats
	for range reconcilePassBatches {
		if err := ctx.Err(); err != nil {
			return total, false, err
		}
		stats, err := reconcile(ctx, recipe, s.store.Identity(), reconcileBatch)
		total.Reclaimed += stats.Reclaimed
		total.Added += stats.Added
		total.Removed += stats.Removed
		total.Stale += stats.Stale
		if err != nil {
			return total, false, err
		}
		s.Kick()
		if stats.Reclaimed < reconcileBatch && stats.Added < reconcileBatch && stats.Removed < reconcileBatch && stats.Stale < reconcileBatch {
			return total, false, nil
		}
	}
	return total, true, nil
}

// intSetting reads an integer setting, falling back to def when it is unset
// or unreadable, and clamps it.
func (s *Service) intSetting(ctx context.Context, key string, def, lo, hi int) int {
	value, err := strconv.Atoi(readSetting(ctx, s.settings, key))
	if err != nil {
		value = def
	}
	return min(max(value, lo), hi)
}

const (
	defaultWorkerCount         = 1
	defaultPriorityWorkerCount = 1
	defaultQueueSize           = 128
	defaultBatchLimit          = 25
	defaultPriorityBatchSize   = 1
	defaultNormalBatchSize     = 2

	chapterThumbnailHDRPolicySetting    = "playback.chapter_thumbnail_hdr_policy"
	chapterThumbnailHDRPolicyDefault    = "best_effort"
	chapterThumbnailHDRPolicyDisabled   = "disabled"
	chapterThumbnailHDRPolicyBestEffort = "best_effort"
)

var trickplayRetrySchedule = []time.Duration{
	15 * time.Minute,
	time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

type FileRepository interface {
	GetByID(ctx context.Context, id int) (*models.MediaFile, error)
	ListMissingTrickplay(ctx context.Context, limit int) ([]*models.MediaFile, error)
	UpdateTrickplayState(ctx context.Context, fileID int, trickplay *models.MediaTrickplay) (*models.MediaFile, error)
}

type FolderRepository interface {
	GetByID(ctx context.Context, id int) (*models.MediaFolder, error)
}

type ObjectStore interface {
	PutObject(ctx context.Context, bucket, key string, data []byte) error
	Bucket() string
}

type TrickplayRequest struct {
	FileID        int
	TargetSeconds *float64
}

// SheetService is Prairie's original seek-preview generator: an in-process
// priority queue that writes sprite sheets into media_files.trickplay and
// backs TrickplayQueuer (QueueFileIDs / QueuePriorityFileAtPosition /
// BackfillMissing). It predates the upstream lease-based Service above and
// was renamed from Service when the two implementations were merged.
type SheetService struct {
	fileRepo   FileRepository
	folderRepo FolderRepository
	settings   SettingsReader
	store      ObjectStore
	ffmpegPath string

	notifyNormal        chan struct{}
	notifyPriority      chan struct{}
	workerCount         int
	priorityWorkerCount int
	priorityBatchSize   int
	normalBatchSize     int

	mu             sync.Mutex
	priorityQueue  []int
	normalQueue    []int
	queuedPriority map[int]TrickplayRequest
	queuedNormal   map[int]TrickplayRequest
	inProgress     map[int]struct{}

	extractSheetFunc func(ctx context.Context, file *models.MediaFile, sheetStart float64, toneMap bool) ([]byte, string, error)
	uploadSheetFunc  func(ctx context.Context, fileID, sheetIndex int, frame []byte) (string, error)
	runFFmpegFunc    func(ctx context.Context, ffmpegPath string, args []string) ([]byte, error)
	clock            func() time.Time
}

func NewSheetService(
	fileRepo FileRepository,
	folderRepo FolderRepository,
	settings SettingsReader,
	store ObjectStore,
	ffmpegPath string,
	workerCount int,
) *SheetService {
	if fileRepo == nil || folderRepo == nil || store == nil {
		return nil
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}

	return &SheetService{
		fileRepo:            fileRepo,
		folderRepo:          folderRepo,
		settings:            settings,
		store:               store,
		ffmpegPath:          ffmpegPath,
		notifyNormal:        make(chan struct{}, defaultQueueSize),
		notifyPriority:      make(chan struct{}, defaultQueueSize),
		workerCount:         workerCount,
		priorityWorkerCount: defaultPriorityWorkerCount,
		priorityBatchSize:   defaultPriorityBatchSize,
		normalBatchSize:     defaultNormalBatchSize,
		queuedPriority:      make(map[int]TrickplayRequest),
		queuedNormal:        make(map[int]TrickplayRequest),
		inProgress:          make(map[int]struct{}),
		clock:               time.Now,
	}
}

func (s *SheetService) Start(ctx context.Context) {
	if s == nil {
		return
	}
	slog.InfoContext(ctx,
		"trickplay service started", "component", "trickplay",
		"workers", s.workerCount,
		"priority_workers", s.priorityWorkerCount,
	)
	for i := 0; i < s.workerCount; i++ {
		go s.worker(ctx, false)
	}
	for i := 0; i < s.priorityWorkerCount; i++ {
		go s.worker(ctx, true)
	}
}

func (s *SheetService) QueueFileIDs(_ context.Context, fileIDs []int) {
	if s == nil {
		return
	}
	for _, fileID := range fileIDs {
		if s.enqueue(TrickplayRequest{FileID: fileID}, false) {
			s.notifyNormalWorker()
		}
	}
}

func (s *SheetService) QueuePriorityFileIDs(_ context.Context, fileIDs []int) {
	if s == nil {
		return
	}
	for _, fileID := range fileIDs {
		if s.enqueue(TrickplayRequest{FileID: fileID}, true) {
			s.notifyPriorityWorker()
			s.notifyNormalWorker()
		}
	}
}

func (s *SheetService) QueuePriorityFileAtPosition(_ context.Context, fileID int, targetSeconds float64) {
	if s == nil {
		return
	}
	target := targetSeconds
	if s.enqueue(TrickplayRequest{FileID: fileID, TargetSeconds: &target}, true) {
		s.notifyPriorityWorker()
		s.notifyNormalWorker()
	}
}

func (s *SheetService) BackfillMissing(ctx context.Context, limit int) (int, error) {
	if s == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	files, err := s.fileRepo.ListMissingTrickplay(ctx, limit)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, file := range files {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		if file == nil {
			continue
		}
		if !s.claim(file.ID) {
			continue
		}
		_, err := s.processRequest(ctx, TrickplayRequest{FileID: file.ID}, false)
		s.finishProcessing(file.ID)
		if err != nil {
			slog.WarnContext(ctx, "trickplay backfill failed", "component", "trickplay", "file_id", file.ID, "error", err)
			continue
		}
		processed++
	}
	return processed, nil
}

// claim marks fileID as in progress so backfill and queue workers cannot
// overwrite each other's trickplay JSONB writes. Returns false when another
// worker already owns the file.
func (s *SheetService) claim(fileID int) bool {
	if s == nil || fileID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.inProgress[fileID]; busy {
		return false
	}
	if _, queued := s.queuedPriority[fileID]; queued {
		return false
	}
	if _, queued := s.queuedNormal[fileID]; queued {
		return false
	}
	s.inProgress[fileID] = struct{}{}
	return true
}

func (s *SheetService) worker(ctx context.Context, priorityOnly bool) {
	for {
		req, ok := s.nextRequest(ctx, priorityOnly)
		if !ok {
			return
		}
		requeueNormal, err := s.processRequest(ctx, req, priorityOnly)
		if err != nil {
			slog.WarnContext(ctx, "trickplay generation failed", "component", "trickplay", "file_id", req.FileID, "error", err)
		}
		notifyPriority, notifyNormal := s.finishProcessing(req.FileID)
		if notifyPriority {
			s.notifyPriorityWorker()
		}
		if notifyNormal {
			s.notifyNormalWorker()
		}
		if requeueNormal && s.enqueue(TrickplayRequest{FileID: req.FileID}, false) {
			s.notifyNormalWorker()
		}
	}
}

func (s *SheetService) processRequest(ctx context.Context, req TrickplayRequest, priority bool) (bool, error) {
	file, err := s.fileRepo.GetByID(ctx, req.FileID)
	if err != nil || file == nil {
		if err == nil {
			slog.InfoContext(ctx, "trickplay request skipped", "component", "trickplay", "file_id", req.FileID, "priority", priority, "reason", "file_not_found")
		}
		return false, err
	}
	if file.Duration <= 0 {
		slog.InfoContext(ctx, "trickplay request skipped", "component", "trickplay", "file_id", req.FileID, "priority", priority, "reason", "no_duration")
		return false, nil
	}

	folder, err := s.folderRepo.GetByID(ctx, file.MediaFolderID)
	if err != nil || folder == nil {
		if err == nil {
			slog.InfoContext(ctx, "trickplay request skipped", "component", "trickplay", "file_id", req.FileID, "priority", priority, "reason", "folder_not_found")
		}
		return false, err
	}
	if !folder.Enabled || !folder.TrickplayEnabled {
		slog.InfoContext(ctx,
			"trickplay request skipped", "component", "trickplay",
			"file_id", req.FileID,
			"priority", priority,
			"reason", "folder_disabled",
			"folder_id", folder.ID,
		)
		return false, nil
	}

	now := s.now()
	if !IsRetryEligible(file.Trickplay, now) {
		slog.InfoContext(ctx,
			"trickplay request skipped", "component", "trickplay",
			"file_id", req.FileID,
			"priority", priority,
			"reason", "file_cooldown",
			"retry_after", file.Trickplay.RetryAfter,
		)
		return false, nil
	}

	hdrPolicy := s.hdrPolicy(ctx)
	if needsTonemap(file) && hdrPolicy == chapterThumbnailHDRPolicyDisabled {
		slog.InfoContext(ctx,
			"trickplay request skipped", "component", "trickplay",
			"file_id", req.FileID,
			"priority", priority,
			"reason", "hdr_policy_disabled",
		)
		return false, nil
	}

	state := ensureTrickplayState(file.Trickplay, file.Duration)
	expectedThumbs := ExpectedThumbnailCount(file.Duration, state.IntervalSeconds)
	tilesPerSheet := TilesPerSheet(state.TileColumns, state.TileRows)
	expectedSheets := ExpectedSheetCount(expectedThumbs, tilesPerSheet)
	if expectedSheets <= 0 {
		return false, nil
	}

	missing := missingSheetIndices(state, expectedSheets)
	if len(missing) == 0 {
		slog.InfoContext(ctx, "trickplay request skipped", "component", "trickplay", "file_id", req.FileID, "priority", priority, "reason", "complete")
		return false, nil
	}

	selected := selectSheetIndices(missing, req.TargetSeconds, state.IntervalSeconds, tilesPerSheet, priority, s.batchSize(priority))
	if len(selected) == 0 {
		return false, nil
	}

	toneMap := needsTonemap(file) && hdrPolicy == chapterThumbnailHDRPolicyBestEffort
	slog.InfoContext(ctx,
		"trickplay processing started", "component", "trickplay",
		"file_id", req.FileID,
		"priority", priority,
		"target_seconds", requestTargetSeconds(req),
		"selected_sheets", selected,
		"expected_sheets", expectedSheets,
		"hdr_policy", hdrPolicy,
	)

	generated := 0
	var lastErr error
	var lastReason string
	for _, sheetIndex := range selected {
		sheetStart := SheetStartSeconds(sheetIndex, state.IntervalSeconds, tilesPerSheet)
		frame, reason, extractErr := s.extractSheet(ctx, file, sheetStart, toneMap)
		if extractErr != nil {
			slog.WarnContext(ctx,
				"trickplay extract failed", "component", "trickplay",
				"file_id", file.ID,
				"sheet_index", sheetIndex,
				"reason", reason,
				"error", extractErr,
			)
			lastErr = extractErr
			lastReason = reason
			break
		}

		path, uploadErr := s.uploadSheet(ctx, file.ID, sheetIndex, frame)
		if uploadErr != nil {
			slog.WarnContext(ctx,
				"trickplay upload failed", "component", "trickplay",
				"file_id", file.ID,
				"sheet_index", sheetIndex,
				"error", uploadErr,
			)
			lastErr = uploadErr
			lastReason = "sheet_upload_failed"
			break
		}

		if state.Height <= 0 {
			if cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(frame)); decodeErr == nil && state.TileRows > 0 {
				state.Height = cfg.Height / state.TileRows
			}
		}
		upsertSheet(state, sheetIndex, path)
		generated++
	}

	state.ThumbnailCount = expectedThumbs
	state.DurationSeconds = file.Duration

	if lastErr != nil {
		nextCount := state.FailureCount + 1
		retryAfter := now.Add(retryDurationForCount(nextCount))
		state.FailureCount = nextCount
		state.RetryAfter = &retryAfter
		state.LastError = failureDetail(lastReason, lastErr)
		if _, persistErr := s.fileRepo.UpdateTrickplayState(ctx, file.ID, state); persistErr != nil {
			return false, persistErr
		}
		slog.InfoContext(ctx,
			"trickplay processing finished", "component", "trickplay",
			"file_id", req.FileID,
			"priority", priority,
			"generated_count", generated,
			"failed", true,
			"requeue", false,
		)
		// Keep failure/backoff state even after partial progress; backfill or a
		// later priority queue will resume once RetryAfter elapses.
		return false, lastErr
	}

	state.RetryAfter = nil
	state.FailureCount = 0
	state.LastError = ""
	if _, persistErr := s.fileRepo.UpdateTrickplayState(ctx, file.ID, state); persistErr != nil {
		return false, persistErr
	}

	requeue := IsIncomplete(state, file.Duration)
	slog.InfoContext(ctx,
		"trickplay processing finished", "component", "trickplay",
		"file_id", req.FileID,
		"priority", priority,
		"generated_count", generated,
		"requeue", requeue,
	)
	return requeue, nil
}

func ensureTrickplayState(existing *models.MediaTrickplay, durationSeconds int) *models.MediaTrickplay {
	if existing == nil || existing.DurationSeconds != durationSeconds || existing.ThumbnailCount == 0 {
		return &models.MediaTrickplay{
			IntervalSeconds: DefaultIntervalSeconds,
			Width:           DefaultTileWidth,
			Height:          0,
			TileColumns:     DefaultTileColumns,
			TileRows:        DefaultTileRows,
			ThumbnailCount:  ExpectedThumbnailCount(durationSeconds, DefaultIntervalSeconds),
			DurationSeconds: durationSeconds,
			Sheets:          nil,
		}
	}
	cp := *existing
	cp.Sheets = append([]models.MediaTrickplaySheet(nil), existing.Sheets...)
	if cp.IntervalSeconds <= 0 {
		cp.IntervalSeconds = DefaultIntervalSeconds
	}
	if cp.Width <= 0 {
		cp.Width = DefaultTileWidth
	}
	if cp.TileColumns <= 0 {
		cp.TileColumns = DefaultTileColumns
	}
	if cp.TileRows <= 0 {
		cp.TileRows = DefaultTileRows
	}
	return &cp
}

func missingSheetIndices(state *models.MediaTrickplay, expectedSheets int) []int {
	present := make(map[int]struct{}, len(state.Sheets))
	for _, sheet := range state.Sheets {
		if sheet.Path == "" {
			continue
		}
		present[sheet.Index] = struct{}{}
	}
	missing := make([]int, 0, expectedSheets)
	for i := 0; i < expectedSheets; i++ {
		if _, ok := present[i]; !ok {
			missing = append(missing, i)
		}
	}
	return missing
}

func selectSheetIndices(
	missing []int,
	targetSeconds *float64,
	interval float64,
	tilesPerSheet int,
	priority bool,
	limit int,
) []int {
	if len(missing) == 0 || limit <= 0 {
		return nil
	}
	if !priority || targetSeconds == nil {
		if len(missing) > limit {
			return append([]int(nil), missing[:limit]...)
		}
		return append([]int(nil), missing...)
	}

	targetSheet := SheetIndex(TileIndex(*targetSeconds, interval), tilesPerSheet)
	type ranked struct {
		index    int
		distance int
	}
	rankedSheets := make([]ranked, 0, len(missing))
	for _, idx := range missing {
		distance := idx - targetSheet
		if distance < 0 {
			distance = -distance
		}
		rankedSheets = append(rankedSheets, ranked{index: idx, distance: distance})
	}
	sort.SliceStable(rankedSheets, func(i, j int) bool {
		if rankedSheets[i].distance == rankedSheets[j].distance {
			return rankedSheets[i].index < rankedSheets[j].index
		}
		return rankedSheets[i].distance < rankedSheets[j].distance
	})
	if len(rankedSheets) > limit {
		rankedSheets = rankedSheets[:limit]
	}
	out := make([]int, 0, len(rankedSheets))
	for _, item := range rankedSheets {
		out = append(out, item.index)
	}
	return out
}

func upsertSheet(state *models.MediaTrickplay, sheetIndex int, path string) {
	for i := range state.Sheets {
		if state.Sheets[i].Index == sheetIndex {
			state.Sheets[i].Path = path
			return
		}
	}
	state.Sheets = append(state.Sheets, models.MediaTrickplaySheet{Index: sheetIndex, Path: path})
	sort.Slice(state.Sheets, func(i, j int) bool {
		return state.Sheets[i].Index < state.Sheets[j].Index
	})
}

func (s *SheetService) extractSheet(ctx context.Context, file *models.MediaFile, sheetStart float64, toneMap bool) ([]byte, string, error) {
	if s.extractSheetFunc != nil {
		return s.extractSheetFunc(ctx, file, sheetStart, toneMap)
	}
	return ExtractSheet(ctx, SheetExtractOptions{
		InputPath:       file.FilePath,
		SheetStart:      sheetStart,
		IntervalSeconds: DefaultIntervalSeconds,
		TileWidth:       DefaultTileWidth,
		TileColumns:     DefaultTileColumns,
		TileRows:        DefaultTileRows,
		FFmpegPath:      s.ffmpegPath,
		ToneMap:         toneMap,
		RunFunc:         s.runFFmpegFunc,
	})
}

func (s *SheetService) uploadSheet(ctx context.Context, fileID, sheetIndex int, frame []byte) (string, error) {
	if s.uploadSheetFunc != nil {
		return s.uploadSheetFunc(ctx, fileID, sheetIndex, frame)
	}

	result, err := imageutil.GenerateVariants(frame, nil)
	if err != nil {
		return "", fmt.Errorf("generate webp: %w", err)
	}

	bucket := s.store.Bucket()
	var originalKey string
	for _, variant := range result.Variants {
		if variant.Key != "original" {
			continue
		}
		key := filepath.ToSlash(fmt.Sprintf("trickplay/%d/%d%s", fileID, sheetIndex, result.Ext))
		if err := s.store.PutObject(ctx, bucket, key, variant.Data); err != nil {
			return "", fmt.Errorf("upload %s: %w", key, err)
		}
		originalKey = key
	}
	if originalKey == "" {
		return "", fmt.Errorf("generate webp: missing original variant")
	}
	return originalKey, nil
}

func (s *SheetService) enqueue(req TrickplayRequest, priority bool) bool {
	if req.FileID <= 0 {
		return false
	}

	s.mu.Lock()
	enqueued := false
	if priority {
		if existing, ok := s.queuedPriority[req.FileID]; ok {
			s.queuedPriority[req.FileID] = mergeRequest(existing, req)
		} else if existing, ok := s.queuedNormal[req.FileID]; ok {
			delete(s.queuedNormal, req.FileID)
			req = mergeRequest(existing, req)
			s.queuedPriority[req.FileID] = req
			s.priorityQueue = append(s.priorityQueue, req.FileID)
			enqueued = true
		} else {
			s.queuedPriority[req.FileID] = req
			s.priorityQueue = append(s.priorityQueue, req.FileID)
			enqueued = true
		}
	} else if _, ok := s.inProgress[req.FileID]; ok {
		// skip
	} else if _, ok := s.queuedPriority[req.FileID]; ok {
		// skip
	} else if _, ok := s.queuedNormal[req.FileID]; ok {
		// skip
	} else {
		s.queuedNormal[req.FileID] = req
		s.normalQueue = append(s.normalQueue, req.FileID)
		enqueued = true
	}
	s.mu.Unlock()
	return enqueued
}

func mergeRequest(existing TrickplayRequest, incoming TrickplayRequest) TrickplayRequest {
	if incoming.TargetSeconds != nil {
		existing.TargetSeconds = incoming.TargetSeconds
	}
	return existing
}

func (s *SheetService) notifyNormalWorker() {
	select {
	case s.notifyNormal <- struct{}{}:
	default:
	}
}

func (s *SheetService) notifyPriorityWorker() {
	select {
	case s.notifyPriority <- struct{}{}:
	default:
	}
}

func (s *SheetService) nextRequest(ctx context.Context, priorityOnly bool) (TrickplayRequest, bool) {
	for {
		s.mu.Lock()
		if req, ok := s.popQueuedLocked(true); ok {
			s.mu.Unlock()
			return req, true
		}
		if !priorityOnly {
			if req, ok := s.popQueuedLocked(false); ok {
				s.mu.Unlock()
				return req, true
			}
		}
		s.mu.Unlock()

		if priorityOnly {
			select {
			case <-ctx.Done():
				return TrickplayRequest{}, false
			case <-s.notifyPriority:
			}
			continue
		}

		select {
		case <-ctx.Done():
			return TrickplayRequest{}, false
		case <-s.notifyPriority:
		case <-s.notifyNormal:
		}
	}
}

func (s *SheetService) finishProcessing(fileID int) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inProgress, fileID)
	_, notifyPriority := s.queuedPriority[fileID]
	_, notifyNormal := s.queuedNormal[fileID]
	return notifyPriority, notifyNormal
}

func (s *SheetService) popQueuedLocked(priority bool) (TrickplayRequest, bool) {
	queue := &s.normalQueue
	queued := s.queuedNormal
	if priority {
		queue = &s.priorityQueue
		queued = s.queuedPriority
	}

	for i, fileID := range *queue {
		req, ok := queued[fileID]
		if !ok {
			continue
		}
		if _, busy := s.inProgress[fileID]; busy {
			continue
		}
		delete(queued, fileID)
		s.inProgress[fileID] = struct{}{}
		*queue = append((*queue)[:i], (*queue)[i+1:]...)
		return req, true
	}
	*queue = compactQueue(*queue, queued)
	return TrickplayRequest{}, false
}

func compactQueue(queue []int, queued map[int]TrickplayRequest) []int {
	if len(queue) == 0 {
		return queue
	}
	compacted := make([]int, 0, len(queue))
	for _, fileID := range queue {
		if _, ok := queued[fileID]; ok {
			compacted = append(compacted, fileID)
		}
	}
	return compacted
}

func requestTargetSeconds(req TrickplayRequest) any {
	if req.TargetSeconds == nil {
		return nil
	}
	return *req.TargetSeconds
}

func (s *SheetService) batchSize(priority bool) int {
	if priority {
		if s.priorityBatchSize > 0 {
			return s.priorityBatchSize
		}
		return defaultPriorityBatchSize
	}
	if s.normalBatchSize > 0 {
		return s.normalBatchSize
	}
	return defaultNormalBatchSize
}

func (s *SheetService) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

func (s *SheetService) hdrPolicy(ctx context.Context) string {
	if s == nil || s.settings == nil {
		return chapterThumbnailHDRPolicyDefault
	}
	value, err := s.settings.Get(ctx, chapterThumbnailHDRPolicySetting)
	if err != nil {
		return chapterThumbnailHDRPolicyDefault
	}
	switch strings.TrimSpace(strings.ToLower(value)) {
	case chapterThumbnailHDRPolicyDisabled:
		return chapterThumbnailHDRPolicyDisabled
	case "", chapterThumbnailHDRPolicyBestEffort:
		return chapterThumbnailHDRPolicyBestEffort
	default:
		return chapterThumbnailHDRPolicyDefault
	}
}

func needsTonemap(file *models.MediaFile) bool {
	if file == nil {
		return false
	}
	if file.HDR {
		return true
	}
	for _, track := range file.VideoTracks {
		if strings.TrimSpace(track.DolbyVision) != "" {
			return true
		}
	}
	return false
}

func retryDurationForCount(failureCount int) time.Duration {
	if failureCount <= 1 {
		return trickplayRetrySchedule[0]
	}
	index := failureCount - 1
	if index >= len(trickplayRetrySchedule) {
		index = len(trickplayRetrySchedule) - 1
	}
	return trickplayRetrySchedule[index]
}

func failureDetail(reason string, err error) string {
	if err == nil {
		return reason
	}
	if reason == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s: %v", reason, err)
}
