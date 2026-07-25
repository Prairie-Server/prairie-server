package plugins

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
	"silo.requests.arr":   {},
	"silo.requests.seerr": {},
}

func isApprovedCommunityPlugin(pluginID string) bool {
	_, ok := approvedCommunityPluginIDs[pluginID]
	return ok
}
