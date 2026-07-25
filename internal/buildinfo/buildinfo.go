package buildinfo

import (
	"runtime/debug"
	"strings"
)

const unavailableDisplay = "unavailable"

var (
	revisionOverride string
	dirtyOverride    string
)

// Info describes the running Prairie build as embedded by Go's VCS metadata.
type Info struct {
	Display   string `json:"display"`
	Revision  string `json:"revision"`
	Dirty     bool   `json:"dirty"`
	VCSTime   string `json:"vcs_time"`
	Available bool   `json:"available"`
}

// Current reads build metadata from the running binary.
func Current() Info {
	overrideRevision, overrideDirty := parseOverrides(revisionOverride, dirtyOverride)

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildInfo(overrideRevision, overrideDirty, "")
	}
	return resolve(info.Settings, overrideRevision, overrideDirty)
}

func resolve(settings []debug.BuildSetting, fallbackRevision string, fallbackDirty bool) Info {
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
		return buildInfo(revision, dirty, vcsTime)
	}

	return buildInfo(fallbackRevision, fallbackDirty, "")
}

func parseOverrides(revision, dirty string) (string, bool) {
	return strings.TrimSpace(revision), strings.EqualFold(strings.TrimSpace(dirty), "true")
}

func buildInfo(revision string, dirty bool, vcsTime string) Info {
	revision = strings.TrimSpace(revision)
	vcsTime = strings.TrimSpace(vcsTime)
	if revision == "" {
		return unavailableInfo()
	}

	display := revision
	if len(display) > 8 {
		display = display[:8]
	}
	if dirty {
		display += "+dirty"
	}

	return Info{
		Display:   display,
		Revision:  revision,
		Dirty:     dirty,
		VCSTime:   vcsTime,
		Available: true,
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
