package buildinfo

import (
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

const unavailableDisplay = "unavailable"

// Update status values returned on GET /admin/system/build.
const (
	UpdateStatusUpToDate        = "up_to_date"
	UpdateStatusUpdateAvailable = "update_available"
	UpdateStatusUnknown         = "unknown"
)

var (
	revisionOverride string
	dirtyOverride    string
	versionOverride  string
	// buildNumberOverride and builtAtOverride carry the CI container identity
	// (upstream's BUILD_NUMBER / BUILD_DATE ldflags). Both are optional.
	buildNumberOverride string
	builtAtOverride     string
)

// Info describes the running Prairie build as embedded by Go's VCS metadata,
// plus optional marketing-version / update-channel fields.
type Info struct {
	Display   string `json:"display"`
	Revision  string `json:"revision"`
	Dirty     bool   `json:"dirty"`
	VCSTime   string `json:"vcs_time"`
	Available bool   `json:"available"`

	// BuildNumber and BuiltAt identify the CI-built container, when stamped.
	BuildNumber uint64 `json:"build_number"`
	BuiltAt     string `json:"built_at"`

	// Version is the stamped marketing semver when the binary was built from
	// a release tag (BUILD_VERSION / versionOverride). Empty for plain SHA
	// builds.
	Version string `json:"version,omitempty"`
	// LatestVersion is the newest GitHub release tag discovered by the
	// update check, when available.
	LatestVersion string `json:"latest_version,omitempty"`
	// UpdateStatus is up_to_date, update_available, or unknown.
	UpdateStatus string `json:"update_status,omitempty"`
	// ChangelogURL points at release notes (specific tag page or /releases).
	ChangelogURL string `json:"changelog_url,omitempty"`
	// ReleaseURL is the latest GitHub release HTML page when known.
	ReleaseURL string `json:"release_url,omitempty"`
}

// Current reads build metadata from the running binary.
func Current() Info {
	overrideRevision, overrideDirty := parseOverrides(revisionOverride, dirtyOverride)
	version := strings.TrimSpace(versionOverride)

	var current Info
	if info, ok := debug.ReadBuildInfo(); ok {
		current = resolve(info.Settings, overrideRevision, overrideDirty, version)
	} else {
		current = buildInfo(overrideRevision, overrideDirty, "", version)
	}
	current.BuildNumber = parseBuildNumber(buildNumberOverride)
	current.BuiltAt = strings.TrimSpace(builtAtOverride)
	return current
}

func parseBuildNumber(value string) uint64 {
	buildNumber, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return buildNumber
}

// UserAgent names this Prairie build for third-party APIs that ask callers to
// identify their application and version, for example "Prairie/ce6a0f53". A build
// without revision metadata reports "Prairie/dev".
func UserAgent() string { return userAgent() }

var userAgent = sync.OnceValue(func() string { return userAgentFor(Current()) })

// userAgentFor keeps only HTTP token characters of the build's display name,
// so a revision injected at build time can never make the header invalid
// and fail every request that carries it.
func userAgentFor(info Info) string {
	version := ""
	if info.Available {
		version = strings.Map(func(r rune) rune {
			if r < 0x80 && (r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
				return r
			}
			return -1
		}, info.Display)
	}
	if version == "" {
		version = "dev"
	}
	return "Prairie/" + version
}

func resolve(settings []debug.BuildSetting, fallbackRevision string, fallbackDirty bool, version string) Info {
	var (
		revision string
		vcsTime  string
		dirty    bool
	)

	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.time":
			vcsTime = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			dirty = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}

	if revision != "" {
		return buildInfo(revision, dirty, vcsTime, version)
	}

	return buildInfo(fallbackRevision, fallbackDirty, "", version)
}

func parseOverrides(revision, dirty string) (string, bool) {
	return strings.TrimSpace(revision), strings.EqualFold(strings.TrimSpace(dirty), "true")
}

func buildInfo(revision string, dirty bool, vcsTime string, version string) Info {
	revision = strings.TrimSpace(revision)
	vcsTime = strings.TrimSpace(vcsTime)
	version = normalizeVersion(version)
	if revision == "" {
		info := unavailableInfo()
		info.Version = version
		return info
	}

	display := revision
	if len(display) > 8 {
		display = display[:8]
	}
	if dirty {
		display += "+dirty"
	}
	// Prefer marketing version in the short display when stamped.
	if version != "" {
		display = version
		if dirty {
			display += "+dirty"
		}
	}

	return Info{
		Display:   display,
		Revision:  revision,
		Dirty:     dirty,
		VCSTime:   vcsTime,
		Available: true,
		Version:   version,
	}
}

func unavailableInfo() Info {
	return Info{
		Display:   unavailableDisplay,
		Revision:  "",
		Dirty:     false,
		VCSTime:   "",
		Available: false,
	}
}

func normalizeVersion(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(text), "v") && len(text) > 1 {
		if text[1] >= '0' && text[1] <= '9' {
			return strings.TrimSpace(text[1:])
		}
	}
	return text
}
