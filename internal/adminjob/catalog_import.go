package adminjob

import "github.com/prairie-server/prairie-server/internal/catalogseed"

type CatalogImportRequest struct {
	SourceBucket  string                    `json:"source_bucket"`
	SourceKey     string                    `json:"source_key"`
	SourceLabel   string                    `json:"source_label,omitempty"`
	CleanupSource bool                      `json:"cleanup_source,omitempty"`
	LocalPath     string                    `json:"local_path,omitempty"`
	RemoteURL     string                    `json:"remote_url,omitempty"`
	Options       catalogseed.ImportOptions `json:"options"`
}
