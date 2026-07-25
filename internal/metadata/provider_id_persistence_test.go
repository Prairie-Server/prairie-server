package metadata

import (
	"context"
	"sync"
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

type fakeProviderIDRepo struct {
	mu          sync.Mutex
	byContentID map[string][]*models.MediaItemProviderID
	lastReplace map[string]map[string]string
}

func newFakeProviderIDRepo() *fakeProviderIDRepo {
	return &fakeProviderIDRepo{
		byContentID: make(map[string][]*models.MediaItemProviderID),
		lastReplace: make(map[string]map[string]string),
	}
}

func (r *fakeProviderIDRepo) GetByContentID(_ context.Context, contentID string) ([]*models.MediaItemProviderID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.byContentID[contentID]
	out := make([]*models.MediaItemProviderID, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		cp := *row
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeProviderIDRepo) ReplaceByContentID(_ context.Context, contentID string, providerIDs map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]string, len(providerIDs))
	for k, v := range providerIDs {
		if isEphemeralProviderIDKey(k) {
			continue
		}
		cp[k] = v
	}
	r.lastReplace[contentID] = cp
	return nil
}

func (r *fakeProviderIDRepo) FindContentIDByProviderIDs(_ context.Context, providerIDs map[string]string, itemType, excludeContentID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for contentID, rows := range r.byContentID {
		if contentID == excludeContentID {
			continue
		}
		for _, row := range rows {
			if row == nil || row.ProviderID == "" {
				continue
			}
			if itemType != "" && row.ItemType != "" && row.ItemType != itemType {
				continue
			}
			if isEphemeralProviderIDKey(row.Provider) {
				continue
			}
			if providerIDs[row.Provider] == row.ProviderID {
				return contentID, nil
			}
		}
	}
	return "", nil
}

func (r *fakeProviderIDRepo) set(contentID string, ids ...*models.MediaItemProviderID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*models.MediaItemProviderID, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			continue
		}
		row := *id
		cp = append(cp, &row)
	}
	r.byContentID[contentID] = cp
}

type fakeStaleIDRepo struct {
	mu          sync.Mutex
	byContentID map[string][]*models.StaleMediaID
}

func newFakeStaleIDRepo() *fakeStaleIDRepo {
	return &fakeStaleIDRepo{byContentID: make(map[string][]*models.StaleMediaID)}
}

func (r *fakeStaleIDRepo) GetByContentID(_ context.Context, contentID string) ([]*models.StaleMediaID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.byContentID[contentID]
	out := make([]*models.StaleMediaID, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		cp := *row
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeStaleIDRepo) Upsert(_ context.Context, contentID, provider, providerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	updated := false
	for i, row := range r.byContentID[contentID] {
		if row == nil || row.Provider != provider {
			continue
		}
		cp := *row
		cp.ProviderID = providerID
		r.byContentID[contentID][i] = &cp
		updated = true
		break
	}
	if !updated {
		r.byContentID[contentID] = append(r.byContentID[contentID], &models.StaleMediaID{
			ContentID:  contentID,
			Provider:   provider,
			ProviderID: providerID,
		})
	}
	return nil
}

func (r *fakeStaleIDRepo) DeleteByContentID(_ context.Context, contentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byContentID, contentID)
	return nil
}

func (r *fakeStaleIDRepo) set(contentID string, ids ...*models.StaleMediaID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*models.StaleMediaID, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			continue
		}
		row := *id
		cp = append(cp, &row)
	}
	r.byContentID[contentID] = cp
}

type capturingMetadataProvider struct {
	mu       sync.Mutex
	lastReq  MetadataRequest
	response *MetadataResult
}

func (p *capturingMetadataProvider) Slug() string { return "capture" }

func (p *capturingMetadataProvider) Name() string { return "capture" }

func (p *capturingMetadataProvider) ForTypes() []string { return []string{"movie", "series"} }

func (p *capturingMetadataProvider) GetMetadata(_ context.Context, req MetadataRequest) (*MetadataResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastReq = req
	if p.response != nil {
		cp := *p.response
		cp.ProviderIDs = copyMap(p.response.ProviderIDs)
		return &cp, nil
	}
	return &MetadataResult{HasMetadata: false}, nil
}

func (p *capturingMetadataProvider) lastRequest() MetadataRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastReq
}

func TestFindExistingByProviderIDsUsesDurableRepository(t *testing.T) {
	h := newTestHarness()
	ctx := context.Background()

	if err := h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: "existing-1",
		Type:      "movie",
		Title:     "Existing Item",
		Year:      2020,
		Status:    "matched",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	}); err != nil {
		t.Fatalf("upsert existing item: %v", err)
	}

	providerRepo := newFakeProviderIDRepo()
	providerRepo.set("existing-1", &models.MediaItemProviderID{
		ContentID:  "existing-1",
		ItemType:   "movie",
		Provider:   "custom",
		ProviderID: "custom-123",
	})
	h.service.providerIDRepo = providerRepo

	item, err := h.service.findExistingByProviderIDs(ctx, map[string]string{"custom": "custom-123"}, "movie", "")
	if err != nil {
		t.Fatalf("findExistingByProviderIDs: %v", err)
	}
	if item == nil || item.ContentID != "existing-1" {
		t.Fatalf("found item = %#v, want existing-1", item)
	}
}

func TestFindExistingByProviderIDsRespectsDurableProviderItemType(t *testing.T) {
	h := newTestHarness()
	ctx := context.Background()

	for _, item := range []*models.MediaItem{
		{
			ContentID: "movie-1",
			Type:      "movie",
			Title:     "Shared ID Movie",
			Year:      1992,
			Status:    "matched",
			Studios:   []string{},
			Networks:  []string{},
			Countries: []string{},
			Genres:    []string{},
		},
		{
			ContentID: "series-1",
			Type:      "series",
			Title:     "Shared ID Series",
			Year:      2008,
			Status:    "matched",
			Studios:   []string{},
			Networks:  []string{},
			Countries: []string{},
			Genres:    []string{},
		},
	} {
		if err := h.itemRepo.Upsert(ctx, item); err != nil {
			t.Fatalf("upsert item %s: %v", item.ContentID, err)
		}
	}

	providerRepo := newFakeProviderIDRepo()
	providerRepo.set("movie-1", &models.MediaItemProviderID{
		ContentID:  "movie-1",
		ItemType:   "movie",
		Provider:   "tmdb",
		ProviderID: "37264",
	})
	providerRepo.set("series-1", &models.MediaItemProviderID{
		ContentID:  "series-1",
		ItemType:   "series",
		Provider:   "tmdb",
		ProviderID: "37264",
	})
	h.service.providerIDRepo = providerRepo

	movie, err := h.service.findExistingByProviderIDs(ctx, map[string]string{"tmdb": "37264"}, "movie", "")
	if err != nil {
		t.Fatalf("findExistingByProviderIDs movie: %v", err)
	}
	if movie == nil || movie.ContentID != "movie-1" {
		t.Fatalf("movie match = %#v, want movie-1", movie)
	}

	series, err := h.service.findExistingByProviderIDs(ctx, map[string]string{"tmdb": "37264"}, "series", "")
	if err != nil {
		t.Fatalf("findExistingByProviderIDs series: %v", err)
	}
	if series == nil || series.ContentID != "series-1" {
		t.Fatalf("series match = %#v, want series-1", series)
	}
}

func TestProcess_LoadsAndPersistsDurableProviderIDs(t *testing.T) {
	h := newTestHarness()
	ctx := context.Background()

	if err := h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: "existing-1",
		Type:      "movie",
		Title:     "Old Title",
		Status:    "matched",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	}); err != nil {
		t.Fatalf("upsert existing item: %v", err)
	}

	providerRepo := newFakeProviderIDRepo()
	providerRepo.set("existing-1", &models.MediaItemProviderID{
		ContentID:  "existing-1",
		ItemType:   "movie",
		Provider:   "custom",
		ProviderID: "custom-123",
	})
	h.service.providerIDRepo = providerRepo

	provider := &capturingMetadataProvider{
		response: &MetadataResult{
			HasMetadata: true,
			Title:       "Updated Title",
			ProviderIDs: map[string]string{
				"custom":    "custom-123",
				"metadb":    "existing-1",
				"_filepath": "/media/existing-1.mkv",
				"oshash":    "deadbeef",
			},
		},
	}

	result, err := h.service.ProcessWithProviders(ctx, ProcessRequest{
		ContentID: "existing-1",
		Language:  "en",
		Mode:      ModeManualRefresh,
	}, []Provider{provider})
	if err != nil {
		t.Fatalf("ProcessWithProviders: %v", err)
	}
	if result == nil || !result.Updated {
		t.Fatalf("result = %#v, want Updated=true", result)
	}

	req := provider.lastRequest()
	if got := req.ProviderIDs["custom"]; got != "custom-123" {
		t.Fatalf("provider request custom id = %q, want custom-123", got)
	}

	providerRepo.mu.Lock()
	replace := providerRepo.lastReplace["existing-1"]
	providerRepo.mu.Unlock()
	if got := replace["custom"]; got != "custom-123" {
		t.Fatalf("persisted custom id = %q, want custom-123", got)
	}
	if _, ok := replace["metadb"]; ok {
		t.Fatal("persisted metadb id unexpectedly")
	}
	if _, ok := replace["_filepath"]; ok {
		t.Fatal("persisted _filepath unexpectedly")
	}
	if _, ok := replace["oshash"]; ok {
		t.Fatal("persisted oshash unexpectedly")
	}

	item, err := h.itemRepo.GetByID(ctx, "existing-1")
	if err != nil {
		t.Fatalf("get updated item: %v", err)
	}
	if item.Title != "Updated Title" {
		t.Fatalf("item title = %q, want Updated Title", item.Title)
	}
}

func TestProcess_InitialMatchSuppressesRecordedStaleProviderIDs(t *testing.T) {
	h := newTestHarness()
	ctx := context.Background()

	if err := h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: "existing-1",
		Type:      "movie",
		Title:     "Old Title",
		Year:      2001,
		Status:    "pending",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	}); err != nil {
		t.Fatalf("upsert existing item: %v", err)
	}

	staleRepo := newFakeStaleIDRepo()
	staleRepo.set("existing-1", &models.StaleMediaID{
		ContentID:  "existing-1",
		Provider:   "tmdb",
		ProviderID: "dead-tmdb-id",
	})
	h.service.staleIDRepo = staleRepo

	provider := &capturingMetadataProvider{
		response: &MetadataResult{
			HasMetadata: true,
			Title:       "Recovered Title",
			ProviderIDs: map[string]string{"metadb": "existing-1"},
		},
	}

	result, err := h.service.ProcessWithProviders(ctx, ProcessRequest{
		ContentID: "existing-1",
		Language:  "en",
		Mode:      ModeInitialMatch,
		Hints: &MatchHints{
			Title:  "Old Title",
			Year:   2001,
			Type:   "movie",
			TmdbID: "dead-tmdb-id",
		},
	}, []Provider{provider})
	if err != nil {
		t.Fatalf("ProcessWithProviders: %v", err)
	}
	if result == nil || !result.Updated {
		t.Fatalf("result = %#v, want Updated=true", result)
	}

	req := provider.lastRequest()
	if got := req.ProviderIDs["tmdb"]; got != "" {
		t.Fatalf("provider request tmdb id = %q, want empty", got)
	}
}

func TestMergeAndPersist_DoesNotReusePendingProviderMatch(t *testing.T) {
	h := newTestHarness()
	ctx := context.Background()

	if err := h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: "pending-existing",
		Type:      "series",
		Title:     "Example Show",
		Year:      2024,
		Status:    "pending",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	}); err != nil {
		t.Fatalf("upsert existing item: %v", err)
	}

	providerRepo := newFakeProviderIDRepo()
	providerRepo.set("pending-existing", &models.MediaItemProviderID{
		ContentID:  "pending-existing",
		ItemType:   "series",
		Provider:   "custom",
		ProviderID: "series-123",
	})
	h.service.providerIDRepo = providerRepo

	result, err := h.service.mergeAndPersist(ctx, ProcessRequest{
		Mode: ModeInitialMatch,
	}, &MetadataResult{
		HasMetadata: true,
		Title:       "Example Show",
		Year:        2024,
		ProviderIDs: map[string]string{"custom": "series-123"},
	}, nil, nil, nil, "series")
	if err != nil {
		t.Fatalf("mergeAndPersist: %v", err)
	}
	if result == nil || result.ContentID == "" {
		t.Fatalf("result = %#v, want non-empty content id", result)
	}
	if result.ContentID == "pending-existing" {
		t.Fatal("expected pending provider-id match to be ignored")
	}
}

func TestMergeAndPersist_DoesNotRebindSkeletonToPendingProviderMatch(t *testing.T) {
	h := newTestHarness()
	ctx := context.Background()

	if err := h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: "pending-source",
		Type:      "series",
		Title:     "Example Show Alt Root",
		Year:      2024,
		Status:    "pending",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	}); err != nil {
		t.Fatalf("upsert source item: %v", err)
	}
	if err := h.itemRepo.Upsert(ctx, &models.MediaItem{
		ContentID: "pending-target",
		Type:      "series",
		Title:     "Example Show",
		Year:      2024,
		Status:    "pending",
		Studios:   []string{},
		Networks:  []string{},
		Countries: []string{},
		Genres:    []string{},
	}); err != nil {
		t.Fatalf("upsert target item: %v", err)
	}

	providerRepo := newFakeProviderIDRepo()
	providerRepo.set("pending-target", &models.MediaItemProviderID{
		ContentID:  "pending-target",
		ItemType:   "series",
		Provider:   "custom",
		ProviderID: "series-123",
	})
	h.service.providerIDRepo = providerRepo

	result, err := h.service.mergeAndPersist(ctx, ProcessRequest{
		ContentID: "pending-source",
		Mode:      ModeInitialMatch,
	}, &MetadataResult{
		HasMetadata: true,
		Title:       "Example Show",
		Year:        2024,
		ProviderIDs: map[string]string{"custom": "series-123"},
	}, nil, nil, nil, "series")
	if err != nil {
		t.Fatalf("mergeAndPersist: %v", err)
	}
	if result == nil || result.ContentID != "pending-source" {
		t.Fatalf("result = %#v, want content_id pending-source", result)
	}
}

func TestIsProvisionalOwnershipStatus_IncludesAmbiguous(t *testing.T) {
	if !isProvisionalOwnershipStatus("ambiguous") {
		t.Fatal("expected ambiguous items to be treated as provisional ownership")
	}
}
