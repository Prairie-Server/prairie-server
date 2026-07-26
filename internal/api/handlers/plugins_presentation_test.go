package handlers

import (
	"testing"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
)

func TestToPluginPresentationJSONPreservesOperatorMetadata(t *testing.T) {
	t.Parallel()

	got := toPluginPresentationJSON(&pluginv1.PluginPresentation{
		DisplayName:         "Example Plugin",
		Summary:             "Explains the example.",
		DescriptionMarkdown: "Longer description.",
		SetupMarkdown:       "Configure the example.",
		HomepageUrl:         "https://example.com",
		SourceUrl:           "https://github.com/prairie-server/example-plugin",
		SupportUrl:          "https://github.com/prairie-server/example-plugin/issues",
		ChangelogUrl:        "https://github.com/prairie-server/example-plugin/releases",
		PublisherName:       "Prairie",
		PublisherUrl:        "https://github.com/prairie-server",
		LicenseSpdx:         "AGPL-3.0-or-later",
	})

	if got == nil {
		t.Fatal("toPluginPresentationJSON() = nil")
	}
	if got.DisplayName != "Example Plugin" || got.Summary != "Explains the example." {
		t.Fatalf("identity fields = %#v", got)
	}
	if got.SourceURL != "https://github.com/prairie-server/example-plugin" {
		t.Fatalf("source_url = %q", got.SourceURL)
	}
	if got.ChangelogURL != "https://github.com/prairie-server/example-plugin/releases" {
		t.Fatalf("changelog_url = %q", got.ChangelogURL)
	}
}

func TestToPluginPresentationJSONKeepsLegacyManifestOptional(t *testing.T) {
	t.Parallel()

	if got := toPluginPresentationJSON(nil); got != nil {
		t.Fatalf("toPluginPresentationJSON(nil) = %#v, want nil", got)
	}
}
