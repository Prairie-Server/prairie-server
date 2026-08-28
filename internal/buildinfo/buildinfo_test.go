package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		settings         []debug.BuildSetting
		overrideRevision string
		overrideDirty    string
		want             Info
	}{
		{
			name: "clean revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "b4c5aae18aa653725ac697b29a05eac797576008"},
				{Key: "vcs.modified", Value: "false"},
				{Key: "vcs.time", Value: "2026-04-05T22:24:40Z"},
			},
			want: Info{
				Display:   "b4c5aae1",
				Revision:  "b4c5aae18aa653725ac697b29a05eac797576008",
				Dirty:     false,
				VCSTime:   "2026-04-05T22:24:40Z",
				Available: true,
			},
		},
		{
			name: "dirty revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "b4c5aae18aa653725ac697b29a05eac797576008"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.time", Value: "2026-04-05T22:24:40Z"},
			},
			want: Info{
				Display:   "b4c5aae1+dirty",
				Revision:  "b4c5aae18aa653725ac697b29a05eac797576008",
				Dirty:     true,
				VCSTime:   "2026-04-05T22:24:40Z",
				Available: true,
			},
		},
		{
			name: "missing revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.time", Value: "2026-04-05T22:24:40Z"},
			},
			want: Info{
				Display:   "unavailable",
				Revision:  "",
				Dirty:     false,
				VCSTime:   "",
				Available: false,
			},
		},
		{
			name: "override revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "false"},
			},
			overrideRevision: "edf2977f5013df08e57a869bf722af4243a0a4fd",
			want: Info{
				Display:   "edf2977f",
				Revision:  "edf2977f5013df08e57a869bf722af4243a0a4fd",
				Dirty:     false,
				VCSTime:   "",
				Available: true,
			},
		},
		{
			name: "override dirty revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "2026-04-05T22:24:40Z"},
			},
			overrideRevision: "edf2977f5013df08e57a869bf722af4243a0a4fd",
			overrideDirty:    "true",
			want: Info{
				Display:   "edf2977f+dirty",
				Revision:  "edf2977f5013df08e57a869bf722af4243a0a4fd",
				Dirty:     true,
				VCSTime:   "",
				Available: true,
			},
		},
		{
			name:             "override short revision",
			overrideRevision: "abc123",
			want: Info{
				Display:   "abc123",
				Revision:  "abc123",
				Dirty:     false,
				VCSTime:   "",
				Available: true,
			},
		},
		{
			name:             "empty override remains unavailable",
			overrideRevision: "   ",
			overrideDirty:    "true",
			want: Info{
				Display:   "unavailable",
				Revision:  "",
				Dirty:     false,
				VCSTime:   "",
				Available: false,
			},
		},
		{
			name: "embedded revision wins over override",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "b4c5aae18aa653725ac697b29a05eac797576008"},
				{Key: "vcs.modified", Value: "false"},
				{Key: "vcs.time", Value: "2026-04-05T22:24:40Z"},
			},
			overrideRevision: "edf2977f5013df08e57a869bf722af4243a0a4fd",
			overrideDirty:    "true",
			want: Info{
				Display:   "b4c5aae1",
				Revision:  "b4c5aae18aa653725ac697b29a05eac797576008",
				Dirty:     false,
				VCSTime:   "2026-04-05T22:24:40Z",
				Available: true,
			},
		},
		{
			name: "short revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.modified", Value: "false"},
			},
			want: Info{
				Display:   "abc123",
				Revision:  "abc123",
				Dirty:     false,
				VCSTime:   "",
				Available: true,
			},
		},
		{
			name: "missing vcs time",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "b4c5aae18aa653725ac697b29a05eac797576008"},
				{Key: "vcs.modified", Value: "false"},
			},
			want: Info{
				Display:   "b4c5aae1",
				Revision:  "b4c5aae18aa653725ac697b29a05eac797576008",
				Dirty:     false,
				VCSTime:   "",
				Available: true,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			overrideRevision, overrideDirty := parseOverrides(tc.overrideRevision, tc.overrideDirty)
			got := resolve(tc.settings, overrideRevision, overrideDirty, "")
			if got != tc.want {
				t.Fatalf("resolve() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
