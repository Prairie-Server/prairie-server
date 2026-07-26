package hdhomerun

import (
	"context"
	"net/url"
	"strings"
)

// ProbeCandidateURLs expands a user-supplied Dispatcharr / HDHR base into
// concrete discover.json endpoints to try (order matters).
func ProbeCandidateURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	u.Fragment = ""
	u.RawQuery = ""

	path := strings.TrimRight(u.Path, "/")
	var candidates []string
	add := func(pathSuffix string) {
		cp := *u
		cp.Path = pathSuffix
		candidates = append(candidates, cp.String())
	}

	switch {
	case strings.HasSuffix(path, "/discover.json"):
		candidates = append(candidates, u.String())
	case strings.HasSuffix(path, "/hdhr"):
		add(path + "/discover.json")
	case path == "" || path == "/":
		add("/hdhr/discover.json")
		add("/discover.json")
	default:
		add(path + "/hdhr/discover.json")
		add(path + "/discover.json")
		add("/hdhr/discover.json")
	}

	// Dispatcharr's HDHR emulation commonly listens on :9191 even when the web
	// UI is on another port.
	if u.Port() != "9191" {
		host := u.Hostname()
		candidates = append(candidates,
			"http://"+host+":9191/hdhr/discover.json",
			"https://"+host+":9191/hdhr/discover.json",
		)
	}

	return uniqueStrings(candidates)
}

// ClassifyKind guesses whether a verified device looks like Dispatcharr's HDHR
// emulation versus a SiliconDust tuner.
func ClassifyKind(info *DeviceInfo, discoverURL string) string {
	if info == nil {
		return "unknown"
	}
	blob := strings.ToLower(strings.Join([]string{
		info.FriendlyName,
		info.ModelNumber,
		info.FirmwareVersion,
		info.BaseURL,
		discoverURL,
	}, " "))
	if strings.Contains(blob, "dispatcharr") || strings.Contains(blob, "/hdhr") {
		return "dispatcharr"
	}
	if strings.HasPrefix(strings.ToLower(info.ModelNumber), "hdhr") ||
		strings.Contains(blob, "hdhomerun") {
		return "hdhomerun"
	}
	return "hdhomerun"
}

func DiscoverURLForBase(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/discover.json"
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ProbeDiscoverURL verifies a discover.json endpoint via HTTP.
func (c *Client) ProbeDiscoverURL(ctx context.Context, discoverURL string) (*DeviceInfo, error) {
	return c.Discover(ctx, discoverURL, "")
}
