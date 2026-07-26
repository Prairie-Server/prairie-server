package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicmanifest "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/manifest"
	"github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/runtime"
)

type runtimeServer struct {
	pluginv1.UnimplementedRuntimeServer
	manifest *pluginv1.PluginManifest
}

type metadataServer struct {
	pluginv1.UnimplementedMetadataProviderServer
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *metadataServer) Search(context.Context, *pluginv1.SearchMetadataRequest) (*pluginv1.SearchMetadataResponse, error) {
	return &pluginv1.SearchMetadataResponse{
		Results: []*pluginv1.ProviderSearchResult{
			{
				ProviderId: "example-1",
				ItemType:   "movie",
				Title:      "Example Title",
			},
		},
	}, nil
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	runtime.Serve(runtime.ServeConfig{
		Servers: runtime.CapabilityServers{
			Runtime:          &runtimeServer{manifest: manifest},
			MetadataProvider: &metadataServer{},
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}

	manifestPath := filepath.Join(filepath.Dir(executablePath), "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest file: %w", err)
	}

	manifest, err := publicmanifest.Load(data)
	if err != nil {
		return nil, fmt.Errorf("load manifest file: %w", err)
	}
	return manifest, nil
}
