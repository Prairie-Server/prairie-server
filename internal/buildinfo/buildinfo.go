package buildinfo

import (
	"runtime/debug"
	"strings"
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
)

// Info describes the running Prairie build as embedded by Go's VCS metadata,
// plus optional marketing-version / update-channel fields.
type Info struct {
	Display   string `json:"display"`
	Revision  string `json:"revision"`
	Dirty     bool   `json:"dirty"`
	VCSTime   string `json:"vcs_time"`
	Available bool   `json:"available"`

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

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildInfo(overrideRevision, overrideDirty, "", version)
	}
	return resolve(info.Settings, overrideRevision, overrideDirty, version)
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
