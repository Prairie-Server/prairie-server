package plugins

import "testing"

func TestLegacyCatalogURLDetection(t *testing.T) {
	t.Parallel()

	official := []string{
		"https://raw.githubusercontent.com/Silo-Server/silo-plugins/main/manifest.json",
		"https://raw.githubusercontent.com/silo-server/silo-plugins/main/manifest.json",
		"https://raw.githubusercontent.com/ContinuumApp/continuum-plugins/main/manifest.json",
	}
	for _, url := range official {
		if !isLegacyOfficialCatalogURL(url) {
			t.Fatalf("isLegacyOfficialCatalogURL(%q) = false, want true", url)
		}
		if isLegacyCommunityCatalogURL(url) {
			t.Fatalf("isLegacyCommunityCatalogURL(%q) = true, want false", url)
		}
	}

	community := "https://raw.githubusercontent.com/Silo-Community/silo-plugins/main/manifest.json"
	if !isLegacyCommunityCatalogURL(community) {
		t.Fatalf("isLegacyCommunityCatalogURL(%q) = false, want true", community)
	}
	if isLegacyOfficialCatalogURL(community) {
		t.Fatalf("isLegacyOfficialCatalogURL(%q) = true, want false", community)
	}

	if isLegacyOfficialCatalogURL(DefaultRepositoryURL) {
		t.Fatalf("canonical official URL must not be treated as legacy")
	}
	if isLegacyCommunityCatalogURL(ApprovedCommunityRepositoryURL) {
		t.Fatalf("canonical community URL must not be treated as legacy")
	}
}
