package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/prairie-server/prairie-server/internal/cache"
	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/config"
	"github.com/prairie-server/prairie-server/internal/mdblist"
	"github.com/prairie-server/prairie-server/internal/recommendations/embeddings"
	"github.com/prairie-server/prairie-server/internal/s3client"
)

type adminSettingsConnectionCheckRequest struct {
	Values    map[string]string `json:"values"`
	DirtyKeys []string          `json:"dirty_keys"`
}

type connectionCheckResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type s3SettingsCheckClient interface {
	HeadBucket(ctx context.Context, bucket string) error
	PutObject(ctx context.Context, bucket, key string, data []byte) error
	GetObject(ctx context.Context, bucket, key string) ([]byte, error)
	DeleteObject(ctx context.Context, bucket, key string) error
}

type redisSettingsCheckClient interface {
	Ping(ctx context.Context) error
	Close() error
}

type embeddingsSettingsCheckClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type mdblistSettingsCheckClient interface {
	Check(ctx context.Context) error
}

type redisSettingsCheckAdapter struct {
	client *redis.Client
}

func (a *redisSettingsCheckAdapter) Ping(ctx context.Context) error {
	return a.client.Ping(ctx).Err()
}

func (a *redisSettingsCheckAdapter) Close() error {
	return a.client.Close()
}

var newAdminS3SettingsCheckClient = func(cfg s3client.BucketConfig) s3SettingsCheckClient {
	return s3client.NewClient(cfg)
}

var newAdminRedisSettingsCheckClient = func(cfg config.RedisConfig) (redisSettingsCheckClient, error) {
	client, err := cache.NewRedisClient(cfg)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, nil
	}
	return &redisSettingsCheckAdapter{client: client}, nil
}

var newAdminEmbeddingsSettingsCheckClient = func(
	cfg embeddings.ClientConfig,
) embeddingsSettingsCheckClient {
	return embeddings.NewClient(cfg)
}

var newAdminMDBListSettingsCheckClient = func(apiKey string) mdblistSettingsCheckClient {
	return mdblist.NewClient(apiKey, nil)
}

func (h *AdminHandler) HandleCheckSettingsConnection(w http.ResponseWriter, r *http.Request) {
	if h.SettingsRepo == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Settings store not configured")
		return
	}

	kind := chi.URLParam(r, "kind")
	if strings.TrimSpace(kind) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "Check kind is required")
		return
	}

	var req adminSettingsConnectionCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid request body")
		return
	}
	if req.Values == nil {
		req.Values = map[string]string{}
	}

	effectiveSettings, err := h.effectiveSettingsForConnectionCheck(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load settings")
		return
	}

	cfg, err := config.LoadFromDB(effectiveSettings)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var response connectionCheckResponse
	switch kind {
	case "s3_public", "s3_operational":
		response = checkS3PublicConnection(r.Context(), cfg)
	case "s3_private":
		response = checkS3PrivateConnection(r.Context(), cfg)
	case "redis":
		response = checkRedisConnection(r.Context(), cfg)
	case "recommendations_embedding":
		response = checkRecommendationsEmbeddingConnection(r.Context(), cfg)
	case "meilisearch":
		response = checkMeilisearchConnection(r.Context(), effectiveSettings)
	case "mdblist":
		response = checkMDBListConnection(r.Context(), cfg)
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "Unsupported connection check kind")
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func checkMDBListConnection(ctx context.Context, cfg *config.Config) connectionCheckResponse {
	if strings.TrimSpace(cfg.MDBListAPIKey) == "" {
		return connectionCheckResponse{Success: false, Message: "MDBList API key is required."}
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := newAdminMDBListSettingsCheckClient(cfg.MDBListAPIKey).Check(checkCtx); err != nil {
		return connectionCheckResponse{Success: false, Message: fmt.Sprintf("MDBList connection check failed: %v", err)}
	}
	return connectionCheckResponse{Success: true, Message: "MDBList API key verified."}
}

func checkMeilisearchConnection(ctx context.Context, settings map[string]string) connectionCheckResponse {
	searchSettings, err := catalog.CatalogSearchSettingsFromMap(settings)
	if err != nil {
		return connectionCheckResponse{Success: false, Message: err.Error()}
	}
	if searchSettings.MeilisearchURL == "" {
		return connectionCheckResponse{Success: false, Message: "Meilisearch URL is required"}
	}
	searchSettings.Provider = catalog.SearchProviderMeilisearch
	indexer := catalog.NewCatalogSearchIndexer(nil, nil)
	if err := indexer.CheckConnection(ctx, searchSettings); err != nil {
		return connectionCheckResponse{Success: false, Message: err.Error()}
	}
	return connectionCheckResponse{Success: true, Message: "Meilisearch connection successful"}
}

func (h *AdminHandler) effectiveSettingsForConnectionCheck(
	ctx context.Context,
	req adminSettingsConnectionCheckRequest,
) (map[string]string, error) {
	settings, err := h.SettingsRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]string, len(settings)+len(h.BootstrapSensitiveValues)+len(req.DirtyKeys))
	for key, value := range settings {
		merged[key] = value
	}
	for key, value := range h.BootstrapSensitiveValues {
		if value == "" {
			continue
		}
		merged[key] = value
	}
	for _, key := range req.DirtyKeys {
		merged[key] = req.Values[key]
	}

	return merged, nil
}

func checkS3PublicConnection(ctx context.Context, cfg *config.Config) connectionCheckResponse {
	if strings.TrimSpace(cfg.S3.Public.Endpoint) == "" {
		return connectionCheckResponse{Success: false, Message: "S3 endpoint is required."}
	}
	if strings.TrimSpace(cfg.S3.Public.Bucket) == "" {
		return connectionCheckResponse{Success: false, Message: "S3 bucket is required."}
	}

	urlAuth := strings.TrimSpace(cfg.S3.Public.URLAuth)
	switch urlAuth {
	case "", s3client.URLAuthPresigned:
	case s3client.URLAuthPublic:
		if strings.TrimSpace(cfg.S3.Public.ReadEndpoint) == "" {
			return connectionCheckResponse{
				Success: false,
				Message: "A public endpoint is required when URL auth is set to Public.",
			}
		}
	case s3client.URLAuthCloudflareToken:
		if strings.TrimSpace(cfg.S3.Public.ReadEndpoint) == "" {
			return connectionCheckResponse{
				Success: false,
				Message: "A public endpoint is required when URL auth is set to Cloudflare Token.",
			}
		}
		if strings.TrimSpace(cfg.S3.Public.TokenSecret) == "" {
			return connectionCheckResponse{
				Success: false,
				Message: "A token secret is required when URL auth is set to Cloudflare Token.",
			}
		}
	default:
		return connectionCheckResponse{
			Success: false,
			Message: fmt.Sprintf("Unsupported S3 URL auth method %q.", urlAuth),
		}
	}

	client := newAdminS3SettingsCheckClient(s3client.BucketConfig{
		Endpoint:       cfg.S3.Public.Endpoint,
		PublicEndpoint: cfg.S3.Public.ReadEndpoint,
		Region:         cfg.S3.Public.Region,
		Bucket:         cfg.S3.Public.Bucket,
		KeyPrefix:      cfg.S3.Public.KeyPrefix,
		AccessKey:      cfg.S3.Public.AccessKey,
		SecretKey:      cfg.S3.Public.SecretKey,
		PathStyle:      cfg.S3.Public.PathStyle,
		URLAuth:        cfg.S3.Public.URLAuth,
		TokenSecret:    cfg.S3.Public.TokenSecret,
		TokenParam:     cfg.S3.Public.TokenParam,
		TokenTTL:       cfg.S3.Public.TokenTTL,
	})

	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.HeadBucket(checkCtx, cfg.S3.Public.Bucket); err != nil {
		return connectionCheckResponse{
			Success: false,
			Message: fmt.Sprintf("S3 connection check failed: %v", err),
		}
	}
	if err := checkS3ObjectPermissions(checkCtx, client, cfg.S3.Public.Bucket); err != nil {
		return connectionCheckResponse{Success: false, Message: fmt.Sprintf("S3 object permission check failed: %v", err)}
	}

	return connectionCheckResponse{
		Success: true,
		Message: "S3 connection and object read/write/delete permissions verified.",
	}
}

func checkS3PrivateConnection(ctx context.Context, cfg *config.Config) connectionCheckResponse {
	if strings.TrimSpace(cfg.S3.Private.Endpoint) == "" {
		return connectionCheckResponse{Success: false, Message: "S3 endpoint is required."}
	}
	if strings.TrimSpace(cfg.S3.Private.Bucket) == "" {
		return connectionCheckResponse{Success: false, Message: "S3 bucket is required."}
	}

	client := newAdminS3SettingsCheckClient(s3client.BucketConfig{
		Endpoint:  cfg.S3.Private.Endpoint,
		Region:    cfg.S3.Private.Region,
		Bucket:    cfg.S3.Private.Bucket,
		KeyPrefix: cfg.S3.Private.KeyPrefix,
		AccessKey: cfg.S3.Private.AccessKey,
		SecretKey: cfg.S3.Private.SecretKey,
		PathStyle: cfg.S3.Private.PathStyle,
	})

	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.HeadBucket(checkCtx, cfg.S3.Private.Bucket); err != nil {
		return connectionCheckResponse{
			Success: false,
			Message: fmt.Sprintf("S3 connection check failed: %v", err),
		}
	}
	if err := checkS3ObjectPermissions(checkCtx, client, cfg.S3.Private.Bucket); err != nil {
		return connectionCheckResponse{Success: false, Message: fmt.Sprintf("S3 object permission check failed: %v", err)}
	}

	return connectionCheckResponse{
		Success: true,
		Message: "S3 connection and object read/write/delete permissions verified.",
	}
}

func checkS3ObjectPermissions(
	ctx context.Context,
	client s3SettingsCheckClient,
	bucket string,
) (resultErr error) {
	key := fmt.Sprintf(".prairie-admin-connection-check/%d", time.Now().UnixNano())
	payload := []byte("prairie-storage-check")
	deleted := false
	defer func() {
		if deleted {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := client.DeleteObject(cleanupCtx, bucket, key); err != nil {
			cleanupErr := fmt.Errorf("cleanup probe object: %w", err)
			if resultErr == nil {
				resultErr = cleanupErr
			} else {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()

	if err := client.PutObject(ctx, bucket, key, payload); err != nil {
		return fmt.Errorf("write probe object: %w", err)
	}

	read, err := client.GetObject(ctx, bucket, key)
	if err != nil {
		return fmt.Errorf("read probe object: %w", err)
	}
	if !bytes.Equal(read, payload) {
		return fmt.Errorf("read probe object returned unexpected content")
	}
	if err := client.DeleteObject(ctx, bucket, key); err != nil {
		return fmt.Errorf("delete probe object: %w", err)
	}
	deleted = true
	return nil
}

func checkRedisConnection(ctx context.Context, cfg *config.Config) connectionCheckResponse {
	if strings.TrimSpace(cfg.Redis.URL) == "" {
		return connectionCheckResponse{Success: false, Message: "Redis URL is required."}
	}
	if err := probeRedisConfig(ctx, cfg.Redis); err != nil {
		return connectionCheckResponse{
			Success: false,
			Message: fmt.Sprintf("Redis connection check failed: %v", err),
		}
	}
	return connectionCheckResponse{
		Success: true,
		Message: "Redis connection successful.",
	}
}

// probeRedisConfig pings Redis so unreachable URLs cannot be persisted and later
// brick startup under a process supervisor.
func probeRedisConfig(ctx context.Context, redisCfg config.RedisConfig) error {
	if strings.TrimSpace(redisCfg.URL) == "" && redisCfg.SentinelMaster == "" {
		return errors.New("redis URL is required")
	}
	client, err := newAdminRedisSettingsCheckClient(redisCfg)
	if err != nil {
		return err
	}
	if client == nil {
		return errors.New("redis URL is required")
	}
	defer client.Close()

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return client.Ping(checkCtx)
}

func checkRecommendationsEmbeddingConnection(
	ctx context.Context,
	cfg *config.Config,
) connectionCheckResponse {
	if strings.TrimSpace(cfg.Recommendations.EmbeddingBaseURL) == "" {
		return connectionCheckResponse{
			Success: false,
			Message: "Embedding base URL is required.",
		}
	}
	if strings.TrimSpace(cfg.Recommendations.EmbeddingModel) == "" {
		return connectionCheckResponse{
			Success: false,
			Message: "Embedding model is required.",
		}
	}

	client := newAdminEmbeddingsSettingsCheckClient(embeddings.ClientConfig{
		BaseURL: cfg.Recommendations.EmbeddingBaseURL,
		Model:   cfg.Recommendations.EmbeddingModel,
		APIKey:  cfg.Recommendations.EmbeddingAuthToken,
	})

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := client.Embed(checkCtx, []string{"prairie connection test"}); err != nil {
		return connectionCheckResponse{
			Success: false,
			Message: fmt.Sprintf("Embedding connection check failed: %v", err),
		}
	}

	return connectionCheckResponse{
		Success: true,
		Message: "Embedding connection successful.",
	}
}
