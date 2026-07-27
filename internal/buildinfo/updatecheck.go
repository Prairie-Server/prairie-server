package buildinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultReleasesLatestURL = "https://api.github.com/repos/Prairie-Server/prairie-server/releases/latest"
	defaultChangelogURL      = "https://github.com/Prairie-Server/prairie-server/releases"
	defaultUpdateCheckTTL    = 15 * time.Minute
	githubUserAgent          = "Prairie-Server"
)

// UpdateChecker fetches the latest GitHub release and caches the result.
type UpdateChecker struct {
	url        string
	changelog  string
	ttl        time.Duration
	httpClient *http.Client

	mu        sync.Mutex
	cached    remoteRelease
	cachedAt  time.Time
	cachedErr error
	hasCache  bool
}

type remoteRelease struct {
	TagName string
	HTMLURL string
}

// DefaultUpdateChecker is used by the admin build endpoint.
var DefaultUpdateChecker = NewUpdateChecker("", "")

// NewUpdateChecker constructs a checker. Empty url/changelog fall back to the
// Prairie-Server/prairie-server GitHub release URLs.
func NewUpdateChecker(releasesLatestURL, changelogURL string) *UpdateChecker {
	if strings.TrimSpace(releasesLatestURL) == "" {
		releasesLatestURL = defaultReleasesLatestURL
	}
	if strings.TrimSpace(changelogURL) == "" {
		changelogURL = defaultChangelogURL
	}
	return &UpdateChecker{
		url:       releasesLatestURL,
		changelog: changelogURL,
		ttl:       defaultUpdateCheckTTL,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// Enrich attaches latest-version / update-status / changelog fields to info.
func (c *UpdateChecker) Enrich(ctx context.Context, info Info) Info {
	if c == nil {
		info.UpdateStatus = UpdateStatusUnknown
		info.ChangelogURL = defaultChangelogURL
		return info
	}

	release, err := c.latest(ctx)
	if err != nil || release.TagName == "" {
		info.UpdateStatus = UpdateStatusUnknown
		info.ChangelogURL = c.changelog
		return info
	}

	latest := normalizeVersion(release.TagName)
	info.LatestVersion = latest
	info.ReleaseURL = release.HTMLURL
	if release.HTMLURL != "" {
		info.ChangelogURL = release.HTMLURL
	} else {
		info.ChangelogURL = c.changelog
	}

	current := strings.TrimSpace(info.Version)
	if current == "" {
		// SHA-only / unstamped builds cannot be compared to a marketing tag.
		info.UpdateStatus = UpdateStatusUnknown
		return info
	}
	if compareSemver(latest, current) > 0 {
		info.UpdateStatus = UpdateStatusUpdateAvailable
		return info
	}
	info.UpdateStatus = UpdateStatusUpToDate
	return info
}

func (c *UpdateChecker) latest(ctx context.Context) (remoteRelease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasCache && time.Since(c.cachedAt) < c.ttl {
		return c.cached, c.cachedErr
	}

	release, err := c.fetch(ctx)
	c.cached = release
	c.cachedErr = err
	c.cachedAt = time.Now()
	c.hasCache = true
	return release, err
}

func (c *UpdateChecker) fetch(ctx context.Context) (remoteRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return remoteRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", githubUserAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return remoteRelease{}, err
	}
	defer resp.Body.Close()

	// No published releases yet.
	if resp.StatusCode == http.StatusNotFound {
		return remoteRelease{}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return remoteRelease{}, fmt.Errorf("github releases: %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return remoteRelease{}, err
	}
	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		tag = strings.TrimSpace(payload.Name)
	}
	return remoteRelease{
		TagName: tag,
		HTMLURL: strings.TrimSpace(payload.HTMLURL),
	}, nil
}

// compareSemver compares two marketing versions. Returns -1 if a < b, 0 if
// equal, 1 if a > b. Non-parseable inputs compare as equal (0) so we avoid
// false "update available" noise.
func compareSemver(a, b string) int {
	pa, okA := parseSemver(a)
	pb, okB := parseSemver(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(raw string) ([3]int, bool) {
	text := normalizeVersion(raw)
	text = strings.SplitN(text, "+", 2)[0]
	text = strings.SplitN(text, "-", 2)[0]
	parts := strings.Split(text, ".")
	if len(parts) < 2 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			out[i] = 0
			continue
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
