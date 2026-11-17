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

// First-party plugin IDs the code names more than once.
const (
	pluginIDTheIntroDB = "prairie.theintrodb"
	pluginIDTVDB       = "prairie.tvdb"
	pluginIDSimkl      = "prairie.watchprovider.simkl"
	pluginIDTrakt      = "prairie.watchprovider.trakt"
)

// officialPluginRepositories binds each first-party plugin ID to the only
// repository whose signed release may install under it. A plugin ID listed
// here gets the required signature policy whichever catalog lists it
// (docs/architecture/plugin-trust.md#release-signatures). prairie-plugins keeps
// a second copy, which its update-catalog workflow checks before it writes
// an entry; adding a first-party plugin changes both.
var officialPluginRepositories = map[string]string{
	"prairie.audiobook-metadata":    "https://github.com/Prairie-Server/prairie-plugin-metadata-audiobook",
	"prairie.auth.ldap":             "https://github.com/Prairie-Server/prairie-plugin-auth-ldap",
	"prairie.auth.oidc":             "https://github.com/Prairie-Server/prairie-plugin-auth-oidc",
	"prairie.autoscan.arr":          "https://github.com/Prairie-Server/prairie-plugin-autoscan-arr",
	"prairie.ebook-metadata":        "https://github.com/Prairie-Server/prairie-plugin-metadata-ebook",
	"silo.manga-metadata":           "https://github.com/Prairie-Server/prairie-plugin-metadata-manga",
	"silo.mdblist":                  "https://github.com/Prairie-Server/silo-plugin-metadata-mdblist",
	"prairie.sportarr":              "https://github.com/Prairie-Server/prairie-plugin-metadata-sportarr",
	pluginIDTheIntroDB:              "https://github.com/Prairie-Server/prairie-plugin-markers-theintrodb",
	pluginIDTMDB:                    "https://github.com/Prairie-Server/silo-plugin-metadata-tmdb",
	pluginIDTVDB:                    "https://github.com/Prairie-Server/prairie-plugin-metadata-tvdb",
	"prairie.watchprovider.floppy":  "https://github.com/Prairie-Server/prairie-plugin-watchprovider-floppy",
	"prairie.watchprovider.mdblist": "https://github.com/Prairie-Server/prairie-plugin-watchprovider-mdblist",
	pluginIDSimkl:                   "https://github.com/Prairie-Server/prairie-plugin-watchprovider-simkl",
	pluginIDTrakt:                   "https://github.com/Prairie-Server/prairie-plugin-watchprovider-trakt",
}

// officialPluginRepository returns the repository bound to a first-party
// plugin ID.
func officialPluginRepository(pluginID string) (string, bool) {
	repo, ok := officialPluginRepositories[pluginID]
	return repo, ok
}
