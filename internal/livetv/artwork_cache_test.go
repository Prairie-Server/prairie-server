package livetv

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/imagecache"
	"github.com/prairie-server/prairie-server/internal/metadata"
)

type stubImageCacher struct {
	mu    sync.Mutex
	calls []imagecache.CacheRequest
	err   error
}

func (s *stubImageCacher) Cache(_ context.Context, req imagecache.CacheRequest) (*imagecache.CacheResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	if s.err != nil {
		return nil, s.err
	}
	imageType := "poster"
	if req.ImageType == metadata.ImageLogo {
		imageType = "logo"
	}
	return &imagecache.CacheResult{
		OriginalPath: fmt.Sprintf("livetv/%s/%s/%s/original.abc.webp", req.ContentType, req.ContentID, imageType),
		Ext:          ".webp",
	}, nil
}

type stubResolver struct {
	failPreferred bool
}

func (s stubResolver) ResolveImageURL(_ context.Context, path string, variant string) string {
	if path == "" {
		return ""
	}
	if s.failPreferred && variant == "card" {
		return ""
	}
	return "https://artwork.example/" + path
}

type stubDeleter struct {
	bucket string
	keys   []string
	err    error
}

func (s *stubDeleter) Bucket() string { return s.bucket }
func (s *stubDeleter) DeleteObjects(_ context.Context, _ string, keys []string) (int, error) {
	s.keys = append([]string(nil), keys...)
	return len(keys), s.err
}

type memoryArtworkIndex struct {
	mu                                                                    sync.Mutex
	nextID                                                                int64
	rows                                                                  map[string]*artworkRow // kind\0subject → row
	lookupErr, upsertErr, readyErr, failErr, touchErr, listErr, deleteErr error
}

func newMemoryArtworkIndex() *memoryArtworkIndex {
	return &memoryArtworkIndex{rows: map[string]*artworkRow{}, nextID: 1}
}

func (m *memoryArtworkIndex) key(kind, subjectID string) string {
	return kind + "\x00" + subjectID
}

func (m *memoryArtworkIndex) LookupMany(_ context.Context, kind string, subjectIDs []string) (map[string]*artworkRow, error) {
	if m.lookupErr != nil {
		return nil, m.lookupErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]*artworkRow{}
	for _, id := range subjectIDs {
		if r := m.rows[m.key(kind, id)]; r != nil {
			cp := *r
			out[id] = &cp
		}
	}
	return out, nil
}

func (m *memoryArtworkIndex) UpsertPending(_ context.Context, kind, subjectID, sourceURL string, expiresAt time.Time) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(kind, subjectID)
	if existing := m.rows[k]; existing != nil && existing.Status == artworkStatusReady &&
		existing.SourceURL == sourceURL && existing.ObjectPath != "" {
		if !expiresAt.IsZero() {
			exp := expiresAt.UTC()
			existing.ExpiresAt = &exp
		}
		return nil
	}
	row := &artworkRow{
		ID:        m.nextID,
		Kind:      kind,
		SubjectID: subjectID,
		SourceURL: sourceURL,
		Status:    artworkStatusPending,
	}
	m.nextID++
	if !expiresAt.IsZero() {
		exp := expiresAt.UTC()
		row.ExpiresAt = &exp
	}
	m.rows[k] = row
	return nil
}

func (m *memoryArtworkIndex) MarkReady(_ context.Context, kind, subjectID, sourceURL, objectPath string, expiresAt time.Time) error {
	if m.readyErr != nil {
		return m.readyErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(kind, subjectID)
	row := m.rows[k]
	if row == nil {
		row = &artworkRow{ID: m.nextID, Kind: kind, SubjectID: subjectID}
		m.nextID++
		m.rows[k] = row
	}
	row.SourceURL = sourceURL
	row.ObjectPath = objectPath
	row.Status = artworkStatusReady
	if !expiresAt.IsZero() {
		exp := expiresAt.UTC()
		row.ExpiresAt = &exp
	}
	return nil
}

func (m *memoryArtworkIndex) MarkFailed(_ context.Context, kind, subjectID, _ string) error {
	if m.failErr != nil {
		return m.failErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(kind, subjectID)
	row := m.rows[k]
	if row == nil {
		row = &artworkRow{ID: m.nextID, Kind: kind, SubjectID: subjectID}
		m.nextID++
		m.rows[k] = row
	}
	row.Status = artworkStatusFailed
	return nil
}

func (m *memoryArtworkIndex) TouchExpiry(_ context.Context, kind, subjectID string, expiresAt time.Time) error {
	if m.touchErr != nil {
		return m.touchErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.rows[m.key(kind, subjectID)]
	if row == nil || row.Status != artworkStatusReady {
		return nil
	}
	exp := expiresAt.UTC()
	if row.ExpiresAt == nil || row.ExpiresAt.Before(exp) {
		row.ExpiresAt = &exp
	}
	return nil
}

func (m *memoryArtworkIndex) ListExpired(_ context.Context, limit int) ([]artworkRow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var out []artworkRow
	for _, r := range m.rows {
		if r.Status != artworkStatusReady || r.ExpiresAt == nil || !r.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, *r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryArtworkIndex) Delete(_ context.Context, id int64) (bool, error) {
	if m.deleteErr != nil {
		return false, m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, r := range m.rows {
		if r.ID == id {
			delete(m.rows, k)
			return true, nil
		}
	}
	return false, nil
}

func TestArtworkCacheEnrichFallsThroughUntilReady(t *testing.T) {
	c := NewArtworkCache(nil, &stubImageCacher{}, stubResolver{})
	channels := []Channel{{ID: "ch1", LogoURL: "https://cdn.example/logo.png"}}
	out := c.EnrichChannels(context.Background(), channels)
	if out[0].LogoURL != "https://cdn.example/logo.png" {
		t.Fatalf("logo_url = %q, want provider URL", out[0].LogoURL)
	}
}

func TestArtworkCacheDisabledSkipsPrograms(t *testing.T) {
	c := NewArtworkCache(nil, &stubImageCacher{}, stubResolver{})
	now := time.Now().UTC()
	programs := []Program{{
		ID:       "p1",
		ImageURL: "https://cdn.example/show.jpg",
		Start:    now.Add(-10 * time.Minute),
		Stop:     now.Add(50 * time.Minute),
	}}
	out := c.EnrichPrograms(context.Background(), programs)
	if out[0].ImageURL != "https://cdn.example/show.jpg" {
		t.Fatalf("image_url = %q", out[0].ImageURL)
	}
}

func TestArtworkCacheRewritesReadyChannelLogo(t *testing.T) {
	idx := newMemoryArtworkIndex()
	_ = idx.MarkReady(context.Background(), ArtworkKindChannelLogo, "ch1",
		"https://cdn.example/logo.png", "livetv/channels/ch1/logo/original.abc.webp", time.Time{})
	cacher := &stubImageCacher{}
	c := newArtworkCache(idx, cacher, stubResolver{})
	c.syncKick = true
	out := c.EnrichChannels(context.Background(), []Channel{{
		ID: "ch1", LogoURL: "https://cdn.example/logo.png",
	}})
	if out[0].LogoURL != "https://artwork.example/livetv/channels/ch1/logo/w500.abc.webp" {
		t.Fatalf("logo_url = %q", out[0].LogoURL)
	}
	if len(cacher.calls) != 0 {
		t.Fatalf("ready row should not re-cache, got %d calls", len(cacher.calls))
	}
}

func TestArtworkCacheKicksChannelLogoCache(t *testing.T) {
	idx := newMemoryArtworkIndex()
	cacher := &stubImageCacher{}
	c := newArtworkCache(idx, cacher, stubResolver{})
	c.syncKick = true
	_ = c.EnrichChannels(context.Background(), []Channel{{
		ID: "ch1", LogoURL: "https://cdn.example/logo.png",
	}})
	if len(cacher.calls) != 1 {
		t.Fatalf("cache calls = %d", len(cacher.calls))
	}
	if cacher.calls[0].ContentType != "channels" || cacher.calls[0].ImageType != metadata.ImageLogo {
		t.Fatalf("unexpected request %+v", cacher.calls[0])
	}
	row := idx.rows[idx.key(ArtworkKindChannelLogo, "ch1")]
	if row == nil || row.Status != artworkStatusReady {
		t.Fatalf("row = %+v", row)
	}
}

func TestArtworkCacheProgramsReadyAndKickAndSkipExpired(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	idx := newMemoryArtworkIndex()
	_ = idx.MarkReady(context.Background(), ArtworkKindProgram, "p-ready",
		"https://cdn.example/ready.jpg", "livetv/programs/p-ready/poster/original.abc.webp", now.Add(time.Hour))
	cacher := &stubImageCacher{}
	c := newArtworkCache(idx, cacher, stubResolver{})
	c.syncKick = true
	c.now = func() time.Time { return now }

	out := c.EnrichPrograms(context.Background(), []Program{
		{ID: "p-ready", ImageURL: "https://cdn.example/ready.jpg", Stop: now.Add(30 * time.Minute)},
		{ID: "p-new", ImageURL: "https://cdn.example/new.jpg", Stop: now.Add(30 * time.Minute)},
		{ID: "p-old", ImageURL: "https://cdn.example/old.jpg", Stop: now.Add(-7 * time.Hour)},
		{ID: "p-blank", ImageURL: "  "},
	})
	if out[0].ImageURL != "https://artwork.example/livetv/programs/p-ready/poster/w500.abc.webp" {
		t.Fatalf("ready rewrite = %q", out[0].ImageURL)
	}
	if out[1].ImageURL != "https://cdn.example/new.jpg" {
		t.Fatalf("pending should keep provider URL until next request, got %q", out[1].ImageURL)
	}
	if out[2].ImageURL != "https://cdn.example/old.jpg" {
		t.Fatalf("expired should not rewrite, got %q", out[2].ImageURL)
	}
	if len(cacher.calls) != 1 || cacher.calls[0].ContentID != "p-new" {
		t.Fatalf("expected one kick for p-new, got %+v", cacher.calls)
	}
}

func TestArtworkCacheLookupErrorFallsThrough(t *testing.T) {
	idx := newMemoryArtworkIndex()
	idx.lookupErr = errors.New("db down")
	c := newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	channels := []Channel{{ID: "ch1", LogoURL: "https://cdn.example/logo.png"}}
	out := c.EnrichChannels(context.Background(), channels)
	if out[0].LogoURL != "https://cdn.example/logo.png" {
		t.Fatalf("logo_url = %q", out[0].LogoURL)
	}
	programs := []Program{{ID: "p1", ImageURL: "https://cdn.example/x.jpg", Stop: time.Now().Add(time.Hour)}}
	pout := c.EnrichPrograms(context.Background(), programs)
	if pout[0].ImageURL != "https://cdn.example/x.jpg" {
		t.Fatalf("image_url = %q", pout[0].ImageURL)
	}
}

func TestArtworkCacheCacheFailureMarksFailed(t *testing.T) {
	idx := newMemoryArtworkIndex()
	cacher := &stubImageCacher{err: errors.New("encode boom")}
	c := newArtworkCache(idx, cacher, stubResolver{})
	c.syncKick = true
	_ = c.EnrichChannels(context.Background(), []Channel{{ID: "ch1", LogoURL: "https://cdn.example/logo.png"}})
	row := idx.rows[idx.key(ArtworkKindChannelLogo, "ch1")]
	if row == nil || row.Status != artworkStatusFailed {
		t.Fatalf("row = %+v", row)
	}
}

func TestArtworkCacheReapExpired(t *testing.T) {
	idx := newMemoryArtworkIndex()
	past := time.Now().UTC().Add(-time.Hour)
	_ = idx.MarkReady(context.Background(), ArtworkKindProgram, "p1",
		"https://cdn.example/x.jpg", "livetv/programs/p1/poster/original.abc.webp", past)
	deleter := &stubDeleter{bucket: "art"}
	c := newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	c.SetObjectDeleter(deleter)
	n, err := c.ReapExpired(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reaped = %d", n)
	}
	if len(deleter.keys) == 0 {
		t.Fatal("expected object deletes")
	}
	if len(idx.rows) != 0 {
		t.Fatalf("rows left = %d", len(idx.rows))
	}
}

func TestArtworkCacheReapErrors(t *testing.T) {
	c := (*ArtworkCache)(nil)
	if n, err := c.ReapExpired(context.Background(), 10); n != 0 || err != nil {
		t.Fatalf("nil cache reap = %d %v", n, err)
	}
	idx := newMemoryArtworkIndex()
	idx.listErr = errors.New("list fail")
	c = newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	if _, err := c.ReapExpired(context.Background(), 5); err == nil {
		t.Fatal("expected list error")
	}
	idx.listErr = nil
	past := time.Now().UTC().Add(-time.Hour)
	_ = idx.MarkReady(context.Background(), ArtworkKindProgram, "p1", "u", "livetv/programs/p1/poster/original.abc.webp", past)
	idx.deleteErr = errors.New("delete fail")
	c.SetObjectDeleter(&stubDeleter{err: errors.New("obj fail")})
	if _, err := c.ReapExpired(context.Background(), 5); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestArtworkCacheResolveFallbackAndSetters(t *testing.T) {
	idx := newMemoryArtworkIndex()
	c := newArtworkCache(idx, &stubImageCacher{}, stubResolver{failPreferred: true})
	c.SetEnabled(false)
	if c.enabled {
		t.Fatal("expected disabled")
	}
	c.SetEnabled(true)
	if !c.enabled {
		t.Fatal("expected enabled")
	}
	c.SetObjectDeleter(&stubDeleter{bucket: "b"})
	url := c.resolve(context.Background(), "livetv/channels/ch1/logo/original.abc.webp")
	if url != "https://artwork.example/livetv/channels/ch1/logo/original.abc.webp" {
		t.Fatalf("resolve = %q", url)
	}
	c2 := newArtworkCache(idx, &stubImageCacher{}, nil)
	if c2.resolve(context.Background(), "x") != "" {
		t.Fatal("nil resolver should return empty")
	}
}

func TestArtworkCacheKickGuardsAndDedupe(t *testing.T) {
	idx := newMemoryArtworkIndex()
	c := newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	c.syncKick = true
	c.kick(ArtworkKindChannelLogo, "", "https://x", time.Time{})
	c.kick(ArtworkKindChannelLogo, "ch1", "", time.Time{})
	c.enabled = false
	c.kick(ArtworkKindChannelLogo, "ch1", "https://x", time.Time{})
	c.enabled = true
	// First call stores inFlight; second concurrent LoadOrStore should no-op.
	c.inFlight.Store(ArtworkKindChannelLogo+"\x00ch1", struct{}{})
	c.kick(ArtworkKindChannelLogo, "ch1", "https://x", time.Time{})
	if len(idx.rows) != 0 {
		t.Fatal("deduped kick should not write")
	}
}

func TestArtworkCacheDeleteObjectsAndUpsertError(t *testing.T) {
	idx := newMemoryArtworkIndex()
	c := newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	if err := c.deleteObjects(context.Background(), "  "); err != nil {
		t.Fatal(err)
	}
	c.SetObjectDeleter(&stubDeleter{bucket: "b"})
	if err := c.deleteObjects(context.Background(), "livetv/channels/ch1/logo/original.abc.webp"); err != nil {
		t.Fatal(err)
	}
	idx.upsertErr = errors.New("upsert fail")
	c.syncKick = true
	_ = c.EnrichChannels(context.Background(), []Channel{{ID: "ch2", LogoURL: "https://cdn.example/logo.png"}})
	if idx.rows[idx.key(ArtworkKindChannelLogo, "ch2")] != nil {
		t.Fatal("upsert failure should not leave a ready row")
	}
}

func TestArtworkCacheNilAndEmptyInputs(t *testing.T) {
	var c *ArtworkCache
	if got := c.EnrichChannels(context.Background(), nil); got != nil {
		t.Fatalf("nil enrich channels = %v", got)
	}
	if got := c.EnrichPrograms(context.Background(), nil); got != nil {
		t.Fatalf("nil enrich programs = %v", got)
	}
	idx := newMemoryArtworkIndex()
	c = newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	if got := c.EnrichChannels(context.Background(), nil); len(got) != 0 {
		t.Fatalf("empty channels = %v", got)
	}
	if got := c.EnrichPrograms(context.Background(), nil); len(got) != 0 {
		t.Fatalf("empty programs = %v", got)
	}
	// Prefer original when Variant cannot rewrite.
	url := c.resolve(context.Background(), "not-a-revisioned-key.webp")
	if url != "https://artwork.example/not-a-revisioned-key.webp" {
		t.Fatalf("resolve = %q", url)
	}
}

func TestTruncateErr(t *testing.T) {
	if truncateErr("  hi  ") != "hi" {
		t.Fatal(truncateErr("  hi  "))
	}
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a'
	}
	if got := truncateErr(string(long)); len(got) != 500 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestServiceArtworkEnrichAndReap(t *testing.T) {
	store := newMemoryStore()
	svc := NewServiceWithStore(store)
	if n, err := svc.ReapExpiredArtwork(context.Background()); n != 0 || err != nil {
		t.Fatalf("nil artwork reap = %d %v", n, err)
	}
	idx := newMemoryArtworkIndex()
	_ = idx.MarkReady(context.Background(), ArtworkKindChannelLogo, "ch1",
		"https://cdn.example/logo.png", "livetv/channels/ch1/logo/original.abc.webp", time.Time{})
	art := newArtworkCache(idx, &stubImageCacher{}, stubResolver{})
	art.syncKick = true
	svc.SetArtworkCache(art)

	_ = store.ReplaceChannelsForTuner(context.Background(), "t1", []Channel{
		{ID: "ch1", Number: "5.1", Callsign: "KING", Name: "KING", LogoURL: "https://cdn.example/logo.png", Enabled: true, StreamURL: "http://tuner/1"},
	})
	channels, err := svc.ListChannels(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].LogoURL != "https://artwork.example/livetv/channels/ch1/logo/w500.abc.webp" {
		t.Fatalf("channels = %+v", channels)
	}
	ch, err := svc.GetChannel(context.Background(), "ch1")
	if err != nil || ch.LogoURL != channels[0].LogoURL {
		t.Fatalf("GetChannel = %+v err=%v", ch, err)
	}

	now := time.Now().UTC()
	_ = store.UpsertPrograms(context.Background(), "src", []Program{{
		ID: "p1", ChannelID: "ch1", Title: "Show", ImageURL: "https://cdn.example/show.jpg",
		Start: now.Add(-time.Minute), Stop: now.Add(time.Hour),
	}})
	_ = idx.MarkReady(context.Background(), ArtworkKindProgram, "p1",
		"https://cdn.example/show.jpg", "livetv/programs/p1/poster/original.abc.webp", now.Add(2*time.Hour))
	programs, err := svc.ListGuide(context.Background(), []string{"ch1"}, now.Add(-time.Hour), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(programs) == 0 || programs[0].ImageURL != "https://artwork.example/livetv/programs/p1/poster/w500.abc.webp" {
		t.Fatalf("programs = %+v", programs)
	}
	prog, err := svc.GetProgram(context.Background(), "p1")
	if err != nil || prog.ImageURL != programs[0].ImageURL {
		t.Fatalf("GetProgram = %+v err=%v", prog, err)
	}
	if n, err := svc.ReapExpiredArtwork(context.Background()); err != nil || n != 0 {
		t.Fatalf("reap = %d %v", n, err)
	}
}

func TestArtworkCacheRequestShape(t *testing.T) {
	req := imagecache.CacheRequest{
		SourceURL:   "https://cdn.example/logo.png",
		ProviderID:  "livetv",
		ContentType: "channels",
		ContentID:   "ch1",
		ImageType:   metadata.ImageLogo,
	}
	if req.ProviderID != "livetv" || req.ContentType != "channels" {
		t.Fatalf("unexpected request %+v", req)
	}
}
