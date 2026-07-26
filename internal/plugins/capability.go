package plugins

import (
	"fmt"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicconvert "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/convert"
)

func CapabilityRecordsFromManifest(manifest *pluginv1.PluginManifest) ([]Capability, error) {
	records, err := publicconvert.CapabilityRecordsFromManifest(manifest)
	if err != nil {
		return nil, err
	}

	hostRecords := make([]Capability, 0, len(records))
	for _, record := range records {
		hostRecords = append(hostRecords, Capability{
			Type:     record.Type,
			ID:       record.ID,
			Metadata: record.Metadata,
		})
	}
	return hostRecords, nil
}

func DecodeCapability(record *Capability) (*pluginv1.CapabilityDescriptor, error) {
	if record == nil {
		return nil, fmt.Errorf("capability record is required")
	}
	return publicconvert.DecodeCapability(publicconvert.CapabilityRecord{
		Type:     record.Type,
		ID:       record.ID,
		Metadata: record.Metadata,
	})
}

func toStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		result := make([]string, 0, len(typed))
		for _, entry := range typed {
			text, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("subscription value must be a string")
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("subscriptions must be a list")
	}
}
