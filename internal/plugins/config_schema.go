package plugins

import (
	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicconfig "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/config"
)

func ValidateGlobalConfigValue(manifest *pluginv1.PluginManifest, key string, value map[string]any) error {
	return publicconfig.ValidateManifestGlobalValue(manifest, key, value)
}
