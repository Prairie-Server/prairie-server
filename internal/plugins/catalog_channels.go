package plugins

import "strings"

const (
	DefaultRepositoryURL  = "https://raw.githubusercontent.com/prairie-server/prairie-plugins/main/manifest.json"
	DefaultRepositoryName = "Prairie maintained"

	ApprovedCommunityRepositoryURL  = "https://raw.githubusercontent.com/Prairie-Community/prairie-plugins/main/manifest.json"
	ApprovedCommunityRepositoryName = "Approved community"

	OfficialRepositoryManagedKey          = "official"
	ApprovedCommunityRepositoryManagedKey = "approved-community"

	RepositorySourcePrairie           = "prairie"
	RepositorySourceApprovedCommunity = "approved_community"
	RepositorySourceExternal          = "external"

	IncludeApprovedCommunityPluginsSetting = "plugins.include_approved_community_plugins"
	MigratedApprovedCommunityCountSetting  = "plugins.approved_community_migrated_plugin_count"
)

// legacyOfficialCatalogURLMarkers are path fragments for Continuum/Silo official
// catalog hosts that must be merged into DefaultRepositoryURL.
var legacyOfficialCatalogURLMarkers = []string{
	"/continuumapp/continuum-plugins/",
	"/silo-server/silo-plugins/",
}

// legacyCommunityCatalogURLMarkers are path fragments for the pre-rebrand
// approved-community catalog host.
var legacyCommunityCatalogURLMarkers = []string{
	"/silo-community/silo-plugins/",
}

func isLegacyOfficialCatalogURL(url string) bool {
	return urlHasAnyMarker(url, legacyOfficialCatalogURLMarkers)
}

func isLegacyCommunityCatalogURL(url string) bool {
	return urlHasAnyMarker(url, legacyCommunityCatalogURLMarkers)
}

func urlHasAnyMarker(url string, markers []string) bool {
	lowered := strings.ToLower(url)
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func legacyCatalogURLMarkersForManagedKey(managedKey string) []string {
	switch managedKey {
	case OfficialRepositoryManagedKey:
		return legacyOfficialCatalogURLMarkers
	case ApprovedCommunityRepositoryManagedKey:
		return legacyCommunityCatalogURLMarkers
	default:
		return nil
	}
}

type managedRepositoryDefinition struct {
	Key         string
	URL         string
	DisplayName string
	SourceKind  string
}

var managedRepositoryDefinitions = []managedRepositoryDefinition{
	{
		Key:         OfficialRepositoryManagedKey,
		URL:         DefaultRepositoryURL,
		DisplayName: DefaultRepositoryName,
		SourceKind:  RepositorySourcePrairie,
	},
	{
		Key:         ApprovedCommunityRepositoryManagedKey,
		URL:         ApprovedCommunityRepositoryURL,
		DisplayName: ApprovedCommunityRepositoryName,
		SourceKind:  RepositorySourceApprovedCommunity,
	},
}

var approvedCommunityPluginIDs = map[string]struct{}{
	"prairie.requests.arr":   {},
	"prairie.requests.seerr": {},
}

func isApprovedCommunityPlugin(pluginID string) bool {
	_, ok := approvedCommunityPluginIDs[pluginID]
	return ok
}
