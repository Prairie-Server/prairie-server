package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/prairie-server/prairie-server/internal/adminjob"
	"github.com/prairie-server/prairie-server/internal/catalogseed"
	"github.com/prairie-server/prairie-server/internal/notifications"
	"github.com/prairie-server/prairie-server/internal/s3client"
)

type CatalogSeedArtifactStore interface {
	AdminJobArtifactStore
	GetObject(ctx context.Context, bucket, key string) ([]byte, error)
	UploadFile(ctx context.Context, bucket, key, path, contentType string) (int64, error)
	ListObjectInfos(ctx context.Context, bucket, prefix string) ([]s3client.ObjectInfo, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	MakeObjectPublic(ctx context.Context, bucket, key string) error
	PublicURL(bucket, key string) (string, error)
}

type CatalogSeedHandler struct {
	service        *catalogseed.Service
	jobRepo        *adminjob.Repository
	store          CatalogSeedArtifactStore
	localImportDir string
	RealtimeHub    *notifications.Hub
}

func NewCatalogSeedHandler(service *catalogseed.Service, jobRepo *adminjob.Repository, store CatalogSeedArtifactStore) *CatalogSeedHandler {
	return &CatalogSeedHandler{service: service, jobRepo: jobRepo, store: store}
}

type exportCatalogSeedRequest struct {
	LibraryIDs []int `json:"library_ids"`
}

type importCatalogSeedResponse struct {
	*catalogseed.ImportResult
}

type catalogSeedImportSource struct {
	Key          string     `json:"key"`
	SizeBytes    int64      `json:"size_bytes"`
	LastModified *time.Time `json:"last_modified,omitempty"`
}

type listCatalogSeedImportSourcesResponse struct {
	Sources []catalogSeedImportSource `json:"sources"`
}

type catalogSeedErrorResponse struct {
	Error          string   `json:"error"`
	Message        string   `json:"message"`
	UnmatchedRoots []string `json:"unmatched_roots,omitempty"`
}

const catalogSeedImportPrefix = "catalog-seeds/"
const remoteCatalogSeedTimeout = 10 * time.Minute
const catalogSeedPublishExpiry = 7 * 24 * time.Hour

func (h *CatalogSeedHandler) HandleExport(w http.ResponseWriter, r *http.Request) {
	var req exportCatalogSeedRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}
	}

	data, err := h.service.Export(r.Context(), catalogseed.ExportOptions{LibraryIDs: req.LibraryIDs})
	if err != nil {
		log.Printf("catalog seed export failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to export catalog seed")
		return
	}

	filename := "prairie-catalog-seed-" + time.Now().UTC().Format("20060102T150405Z") + ".json.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *CatalogSeedHandler) HandleCreateExportJob(w http.ResponseWriter, r *http.Request) {
	if h.jobRepo == nil || h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Catalog export jobs require the private internal S3 bucket")
		return
	}

	var req exportCatalogSeedRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
			return
		}
	}

	job, err := h.jobRepo.Create(r.Context(), adminjob.CreateJobInput{
		JobType:         adminjob.JobTypeCatalogExport,
		CreatedByUserID: currentAdminUserID(r),
		RequestPayload: catalogseed.ExportOptions{
			LibraryIDs: req.LibraryIDs,
		},
		Message: "Queued catalog export",
	})
	if err != nil {
		var conflict *adminjob.ActiveJobConflictError
		switch {
		case errors.As(err, &conflict):
			var jobsHandler *AdminJobsHandler
			if h.jobRepo != nil {
				jobsHandler = NewAdminJobsHandler(h.jobRepo, h.store)
			}
			writeAdminJobConflict(w, "A catalog export is already queued or running", conflict.Job, jobsHandler, r)
		default:
			log.Printf("catalog seed export job creation failed: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to queue catalog export")
		}
		return
	}

	jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
	if h.RealtimeHub != nil {
		publishEventJob(r.Context(), h.RealtimeHub.EventsHub(), "job.created", job)
	}
	writeJSON(w, http.StatusAccepted, adminJobToResponse(r, job, jobsHandler.store))
}

func (h *CatalogSeedHandler) HandlePublishExportJob(w http.ResponseWriter, r *http.Request) {
	if h.jobRepo == nil || h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Catalog export publishing requires the private internal S3 bucket")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "Job ID is required")
		return
	}

	job, err := h.jobRepo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, adminjob.ErrJobNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Job not found")
			return
		}
		log.Printf("catalog export publish load failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load catalog export job")
		return
	}

	if job.JobType != adminjob.JobTypeCatalogExport {
		writeError(w, http.StatusBadRequest, "bad_request", "Only catalog export jobs can be published")
		return
	}
	if job.Status != adminjob.StatusCompleted {
		writeError(w, http.StatusBadRequest, "bad_request", "Only completed catalog export jobs can be published")
		return
	}
	if job.ArtifactBucket == "" || job.ArtifactKey == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "Catalog export job does not have an artifact to publish")
		return
	}
	if job.PublicURL != "" {
		writeJSON(w, http.StatusOK, adminJobToResponse(r, job, h.store))
		return
	}

	publicURL, err := h.store.PresignGetURL(r.Context(), job.ArtifactBucket, job.ArtifactKey, catalogSeedPublishExpiry)
	if err != nil {
		log.Printf("catalog export publish failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Failed to create catalog export URL: %v", err))
		return
	}

	publishedAt := time.Now().UTC()
	if err := h.jobRepo.MarkPublic(r.Context(), job.ID, publicURL, publishedAt); err != nil {
		if errors.Is(err, adminjob.ErrJobNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Job not found")
			return
		}
		log.Printf("catalog export mark public failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to persist catalog export public URL")
		return
	}

	job.PublicURL = publicURL
	job.PublishedAt = &publishedAt
	writeJSON(w, http.StatusOK, adminJobToResponse(r, job, h.store))
}

func (h *CatalogSeedHandler) HandleListImportSources(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "catalog imports from S3 require the private internal S3 bucket")
		return
	}

	objects, err := h.store.ListObjectInfos(r.Context(), h.store.Bucket(), catalogSeedImportPrefix)
	if err != nil {
		log.Printf("catalog seed import source listing failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list catalog seed import sources")
		return
	}

	sources := make([]catalogSeedImportSource, 0, len(objects))
	for _, obj := range objects {
		if !strings.HasSuffix(strings.ToLower(obj.Key), ".json.gz") {
			continue
		}
		sources = append(sources, catalogSeedImportSource{
			Key:          obj.Key,
			SizeBytes:    obj.SizeBytes,
			LastModified: obj.LastModified,
		})
	}

	sort.Slice(sources, func(i, j int) bool {
		left := sources[i].LastModified
		right := sources[j].LastModified
		switch {
		case left == nil && right == nil:
			return sources[i].Key > sources[j].Key
		case left == nil:
			return false
		case right == nil:
			return true
		case left.Equal(*right):
			return sources[i].Key > sources[j].Key
		default:
			return left.After(*right)
		}
	})

	writeJSON(w, http.StatusOK, listCatalogSeedImportSourcesResponse{Sources: sources})
}

func (h *CatalogSeedHandler) HandleCreateImportJob(w http.ResponseWriter, r *http.Request) {
	if h.jobRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Job repository is not configured")
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid multipart form")
		return
	}

	opts, err := parseCatalogImportOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	localPath := r.FormValue("local_path")
	if localPath != "" {
		cleaned := filepath.Clean(localPath)
		abs, absErr := filepath.Abs(cleaned)
		if absErr != nil || !strings.HasSuffix(strings.ToLower(abs), ".json.gz") {
			writeError(w, http.StatusBadRequest, "bad_request", "local path must point to a .json.gz file")
			return
		}
		info, statErr := os.Stat(abs)
		if statErr != nil || !info.Mode().IsRegular() {
			writeError(w, http.StatusBadRequest, "bad_request", "Local file not found or not a regular file")
			return
		}

		job, err := h.jobRepo.Create(r.Context(), adminjob.CreateJobInput{
			JobType:         adminjob.JobTypeCatalogImport,
			CreatedByUserID: currentAdminUserID(r),
			RequestPayload: adminjob.CatalogImportRequest{
				LocalPath:   abs,
				SourceLabel: filepath.Base(abs),
				Options:     opts,
			},
			Message: "Queued catalog import from local file",
		})
		if err != nil {
			var conflict *adminjob.ActiveJobConflictError
			if errors.As(err, &conflict) {
				jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
				writeAdminJobConflict(w, "A catalog import is already queued or running", conflict.Job, jobsHandler, r)
			} else {
				log.Printf("catalog import job creation failed: %v", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to queue catalog import")
			}
			return
		}

		jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
		if h.RealtimeHub != nil {
			publishEventJob(r.Context(), h.RealtimeHub.EventsHub(), "job.created", job)
		}
		writeJSON(w, http.StatusAccepted, adminJobToResponse(r, job, jobsHandler.store))
		return
	}

	remoteURL := r.FormValue("remote_url")
	if remoteURL != "" {
		if _, err := readImportDataFromRemoteURL(r.Context(), remoteURL); err != nil {
			if errors.Is(err, errCatalogSeedImportInvalidRemoteURL) {
				writeError(w, http.StatusBadRequest, "bad_request", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "bad_request", "Failed to load catalog seed source")
			return
		}

		job, err := h.jobRepo.Create(r.Context(), adminjob.CreateJobInput{
			JobType:         adminjob.JobTypeCatalogImport,
			CreatedByUserID: currentAdminUserID(r),
			RequestPayload: adminjob.CatalogImportRequest{
				RemoteURL:   remoteURL,
				SourceLabel: remoteURL,
				Options:     opts,
			},
			Message: "Queued catalog import from remote URL",
		})
		if err != nil {
			var conflict *adminjob.ActiveJobConflictError
			if errors.As(err, &conflict) {
				jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
				writeAdminJobConflict(w, "A catalog import is already queued or running", conflict.Job, jobsHandler, r)
			} else {
				log.Printf("catalog import remote job creation failed: %v", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to queue catalog import")
			}
			return
		}

		jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
		if h.RealtimeHub != nil {
			publishEventJob(r.Context(), h.RealtimeHub.EventsHub(), "job.created", job)
		}
		writeJSON(w, http.StatusAccepted, adminJobToResponse(r, job, jobsHandler.store))
		return
	}

	// S3-based sources — require store
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Catalog import jobs require the private internal S3 bucket")
		return
	}

	sourceBucket, sourceKey, sourceLabel, cleanupSource, err := h.resolveImportJobSource(r)
	if err != nil {
		switch {
		case errors.Is(err, errCatalogSeedImportSourceRequired), errors.Is(err, errCatalogSeedImportSourceConflict):
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		case errors.Is(err, errCatalogSeedImportSourceUnavailable):
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", err.Error())
		case errors.Is(err, adminjob.ErrJobNotFound), errors.Is(err, s3client.ErrNotFound):
			writeError(w, http.StatusBadRequest, "bad_request", "Catalog export artifact not found")
		default:
			log.Printf("catalog import job source failed: %v", err)
			writeError(w, http.StatusBadRequest, "bad_request", "Failed to load catalog seed source")
		}
		return
	}

	job, err := h.jobRepo.Create(r.Context(), adminjob.CreateJobInput{
		JobType:         adminjob.JobTypeCatalogImport,
		CreatedByUserID: currentAdminUserID(r),
		RequestPayload: adminjob.CatalogImportRequest{
			SourceBucket:  sourceBucket,
			SourceKey:     sourceKey,
			SourceLabel:   sourceLabel,
			CleanupSource: cleanupSource,
			Options:       opts,
		},
		Message: "Queued catalog import",
	})
	if err != nil {
		if cleanupSource {
			_ = h.store.DeleteObject(context.Background(), sourceBucket, sourceKey)
		}
		var conflict *adminjob.ActiveJobConflictError
		switch {
		case errors.As(err, &conflict):
			jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
			writeAdminJobConflict(w, "A catalog import is already queued or running", conflict.Job, jobsHandler, r)
		default:
			log.Printf("catalog import job creation failed: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to queue catalog import")
		}
		return
	}

	jobsHandler := NewAdminJobsHandler(h.jobRepo, h.store)
	if h.RealtimeHub != nil {
		publishEventJob(r.Context(), h.RealtimeHub.EventsHub(), "job.created", job)
	}
	writeJSON(w, http.StatusAccepted, adminJobToResponse(r, job, jobsHandler.store))
}

func (h *CatalogSeedHandler) HandleListLocalImportSources(w http.ResponseWriter, r *http.Request) {
	dir := h.localImportDirectory()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, listCatalogSeedImportSourcesResponse{Sources: []catalogSeedImportSource{}})
			return
		}
		log.Printf("listing local import sources in %s: %v", dir, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list local import sources")
		return
	}

	sources := make([]catalogSeedImportSource, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime().UTC()
		sources = append(sources, catalogSeedImportSource{
			Key:          filepath.Join(dir, entry.Name()),
			SizeBytes:    info.Size(),
			LastModified: &modTime,
		})
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].LastModified == nil || sources[j].LastModified == nil {
			return sources[i].Key > sources[j].Key
		}
		return sources[i].LastModified.After(*sources[j].LastModified)
	})

	writeJSON(w, http.StatusOK, listCatalogSeedImportSourcesResponse{Sources: sources})
}

func (h *CatalogSeedHandler) HandleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid multipart form")
		return
	}

	data, err := h.readImportData(r)
	if err != nil {
		switch {
		case errors.Is(err, errCatalogSeedImportSourceRequired), errors.Is(err, errCatalogSeedImportSourceConflict):
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		case errors.Is(err, errCatalogSeedImportInvalidRemoteURL):
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		case errors.Is(err, errCatalogSeedImportInvalidLocalPath):
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		case errors.Is(err, errCatalogSeedImportSourceUnavailable):
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", err.Error())
		case errors.Is(err, adminjob.ErrJobNotFound), errors.Is(err, s3client.ErrNotFound):
			writeError(w, http.StatusBadRequest, "bad_request", "Catalog export artifact not found")
		default:
			log.Printf("catalog seed import source failed: %v", err)
			writeError(w, http.StatusBadRequest, "bad_request", "Failed to load catalog seed source")
		}
		return
	}

	opts, err := parseCatalogImportOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	result, err := h.service.Import(r.Context(), data, opts)
	if err != nil {
		var unmatched *catalogseed.UnmatchedRootsError
		switch {
		case errors.As(err, &unmatched):
			writeCatalogSeedError(w, http.StatusBadRequest, "path_rewrite_required", "Catalog seed import requires additional path rewrites", unmatched.Roots)
		case errors.Is(err, catalogseed.ErrInvalidBundle):
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid catalog seed bundle")
		case errors.Is(err, catalogseed.ErrUnsupportedBundleVersion):
			writeError(w, http.StatusBadRequest, "bad_request", "Unsupported catalog seed version")
		case errors.Is(err, catalogseed.ErrInvalidConflictMode):
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid conflict mode")
		default:
			log.Printf("catalog seed import failed: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to import catalog seed")
		}
		return
	}

	writeJSON(w, http.StatusOK, importCatalogSeedResponse{ImportResult: result})
}

func writeCatalogSeedError(w http.ResponseWriter, status int, code, message string, unmatchedRoots []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(catalogSeedErrorResponse{
		Error:          code,
		Message:        message,
		UnmatchedRoots: unmatchedRoots,
	})
}

const defaultLocalImportDir = "/catalog-seeds"

var (
	errCatalogSeedImportSourceRequired    = errors.New("provide exactly one source: local_path, export_job_id, artifact_key, or remote_url")
	errCatalogSeedImportSourceConflict    = errors.New("provide only one catalog seed source")
	errCatalogSeedImportSourceUnavailable = errors.New("Catalog imports from S3 require the private internal S3 bucket")
	errCatalogSeedImportInvalidLocalPath  = errors.New("Local path must point to an existing .json.gz file")
	errCatalogSeedImportInvalidRemoteURL  = errors.New("remote URL must point to an http(s) .json.gz file")
)

func (h *CatalogSeedHandler) SetLocalImportDir(dir string) {
	h.localImportDir = dir
}

func (h *CatalogSeedHandler) localImportDirectory() string {
	if h.localImportDir != "" {
		return h.localImportDir
	}
	return defaultLocalImportDir
}

func readLocalImportFile(localPath string) ([]byte, error) {
	cleaned := filepath.Clean(localPath)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return nil, errCatalogSeedImportInvalidLocalPath
	}
	if !strings.HasSuffix(strings.ToLower(abs), ".json.gz") {
		return nil, errCatalogSeedImportInvalidLocalPath
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errCatalogSeedImportInvalidLocalPath
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("reading local import file: %w", err)
	}
	return data, nil
}

func (h *CatalogSeedHandler) readImportData(r *http.Request) ([]byte, error) {
	localPath := r.FormValue("local_path")
	jobID := r.FormValue("export_job_id")
	artifactKey := r.FormValue("artifact_key")
	remoteURL := r.FormValue("remote_url")
	hasLocal := localPath != ""
	hasJob := jobID != ""
	hasArtifact := artifactKey != ""
	hasRemoteURL := remoteURL != ""
	sourceCount := 0
	if hasLocal {
		sourceCount++
	}
	if hasJob {
		sourceCount++
	}
	if hasArtifact {
		sourceCount++
	}
	if hasRemoteURL {
		sourceCount++
	}

	switch {
	case sourceCount == 0:
		return nil, errCatalogSeedImportSourceRequired
	case sourceCount > 1:
		return nil, errCatalogSeedImportSourceConflict
	case hasLocal:
		return readLocalImportFile(localPath)
	case hasRemoteURL:
		return readImportDataFromRemoteURL(r.Context(), remoteURL)
	case hasArtifact:
		return h.readImportDataFromArtifactKey(r.Context(), artifactKey)
	default:
		return h.readImportDataFromExportJob(r.Context(), jobID)
	}
}

func (h *CatalogSeedHandler) readImportDataFromExportJob(ctx context.Context, jobID string) ([]byte, error) {
	bucket, key, err := h.resolveExportJobArtifactRef(ctx, jobID)
	if err != nil {
		return nil, err
	}
	data, err := h.store.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (h *CatalogSeedHandler) readImportDataFromArtifactKey(ctx context.Context, artifactKey string) ([]byte, error) {
	if h.store == nil {
		return nil, errCatalogSeedImportSourceUnavailable
	}
	return h.store.GetObject(ctx, h.store.Bucket(), artifactKey)
}

func readImportDataFromRemoteURL(ctx context.Context, remoteURL string) ([]byte, error) {
	parsed, err := url.Parse(remoteURL)
	if err != nil {
		return nil, errCatalogSeedImportInvalidRemoteURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errCatalogSeedImportInvalidRemoteURL
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), ".json.gz") {
		return nil, errCatalogSeedImportInvalidRemoteURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building remote catalog seed request: %w", err)
	}

	client := &http.Client{Timeout: remoteCatalogSeedTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading remote catalog seed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading remote catalog seed: unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading remote catalog seed: %w", err)
	}
	return data, nil
}

func (h *CatalogSeedHandler) resolveImportJobSource(r *http.Request) (bucket string, key string, label string, cleanup bool, err error) {
	jobID := r.FormValue("export_job_id")
	artifactKey := r.FormValue("artifact_key")
	remoteURL := r.FormValue("remote_url")
	hasJob := jobID != ""
	hasArtifact := artifactKey != ""
	hasRemoteURL := remoteURL != ""
	sourceCount := 0
	if hasJob {
		sourceCount++
	}
	if hasArtifact {
		sourceCount++
	}
	if hasRemoteURL {
		sourceCount++
	}

	switch {
	case sourceCount == 0:
		return "", "", "", false, errCatalogSeedImportSourceRequired
	case sourceCount > 1:
		return "", "", "", false, errCatalogSeedImportSourceConflict
	case hasRemoteURL:
		return "", "", remoteURL, false, errCatalogSeedImportInvalidRemoteURL
	case hasArtifact:
		if h.store == nil {
			return "", "", "", false, errCatalogSeedImportSourceUnavailable
		}
		return h.store.Bucket(), artifactKey, filepath.Base(artifactKey), false, nil
	default:
		bucket, key, resolveErr := h.resolveExportJobArtifactRef(r.Context(), jobID)
		if resolveErr != nil {
			return "", "", "", false, resolveErr
		}
		return bucket, key, "Export job " + jobID, false, nil
	}
}

func (h *CatalogSeedHandler) resolveExportJobArtifactRef(ctx context.Context, jobID string) (string, string, error) {
	if h.jobRepo == nil || h.store == nil {
		return "", "", errCatalogSeedImportSourceUnavailable
	}

	job, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return "", "", err
	}
	if job.JobType != adminjob.JobTypeCatalogExport || job.Status != adminjob.StatusCompleted || job.ArtifactBucket == "" || job.ArtifactKey == "" {
		return "", "", fmt.Errorf("catalog export job %s is not ready for import", jobID)
	}
	return job.ArtifactBucket, job.ArtifactKey, nil
}

func parseCatalogImportOptions(r *http.Request) (catalogseed.ImportOptions, error) {
	var rewrites []catalogseed.PathRewrite
	if raw := r.FormValue("path_rewrites"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &rewrites); err != nil {
			return catalogseed.ImportOptions{}, errors.New("invalid path_rewrites")
		}
	}

	return catalogseed.ImportOptions{
		ConflictMode: catalogseed.ConflictMode(r.FormValue("conflict_mode")),
		PathRewrites: rewrites,
	}, nil
}
