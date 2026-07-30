package trickplay

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prairie-server/prairie-server/internal/imageutil"
	"github.com/prairie-server/prairie-server/internal/models"
)

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

type SettingsReader interface {
	Get(ctx context.Context, key string) (string, error)
}

type ObjectStore interface {
	PutObject(ctx context.Context, bucket, key string, data []byte) error
	Bucket() string
}

type TrickplayRequest struct {
	FileID        int
	TargetSeconds *float64
}

type Service struct {
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

func NewService(
	fileRepo FileRepository,
	folderRepo FolderRepository,
	settings SettingsReader,
	store ObjectStore,
	ffmpegPath string,
	workerCount int,
) *Service {
	if fileRepo == nil || folderRepo == nil || store == nil {
		return nil
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}

	return &Service{
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

func (s *Service) Start(ctx context.Context) {
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

func (s *Service) QueueFileIDs(_ context.Context, fileIDs []int) {
	if s == nil {
		return
	}
	for _, fileID := range fileIDs {
		if s.enqueue(TrickplayRequest{FileID: fileID}, false) {
			s.notifyNormalWorker()
		}
	}
}

func (s *Service) QueuePriorityFileIDs(_ context.Context, fileIDs []int) {
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

func (s *Service) QueuePriorityFileAtPosition(_ context.Context, fileID int, targetSeconds float64) {
	if s == nil {
		return
	}
	target := targetSeconds
	if s.enqueue(TrickplayRequest{FileID: fileID, TargetSeconds: &target}, true) {
		s.notifyPriorityWorker()
		s.notifyNormalWorker()
	}
}

func (s *Service) BackfillMissing(ctx context.Context, limit int) (int, error) {
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
func (s *Service) claim(fileID int) bool {
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

func (s *Service) worker(ctx context.Context, priorityOnly bool) {
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

func (s *Service) processRequest(ctx context.Context, req TrickplayRequest, priority bool) (bool, error) {
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

func (s *Service) extractSheet(ctx context.Context, file *models.MediaFile, sheetStart float64, toneMap bool) ([]byte, string, error) {
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

func (s *Service) uploadSheet(ctx context.Context, fileID, sheetIndex int, frame []byte) (string, error) {
	if s.uploadSheetFunc != nil {
		return s.uploadSheetFunc(ctx, fileID, sheetIndex, frame)
	}

	result, err := imageutil.GenerateWebPVariants(frame, nil)
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

func (s *Service) enqueue(req TrickplayRequest, priority bool) bool {
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

func (s *Service) notifyNormalWorker() {
	select {
	case s.notifyNormal <- struct{}{}:
	default:
	}
}

func (s *Service) notifyPriorityWorker() {
	select {
	case s.notifyPriority <- struct{}{}:
	default:
	}
}

func (s *Service) nextRequest(ctx context.Context, priorityOnly bool) (TrickplayRequest, bool) {
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

func (s *Service) finishProcessing(fileID int) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inProgress, fileID)
	_, notifyPriority := s.queuedPriority[fileID]
	_, notifyNormal := s.queuedNormal[fileID]
	return notifyPriority, notifyNormal
}

func (s *Service) popQueuedLocked(priority bool) (TrickplayRequest, bool) {
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

func (s *Service) batchSize(priority bool) int {
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

func (s *Service) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) hdrPolicy(ctx context.Context) string {
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
