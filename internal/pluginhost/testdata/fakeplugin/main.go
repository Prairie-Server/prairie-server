package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicmanifest "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/manifest"
	"github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/runtime"
)

type runtimeServer struct {
	pluginv1.UnimplementedRuntimeServer
	manifest *pluginv1.PluginManifest
	mu       sync.RWMutex
	config   map[string]map[string]any
}

type metadataServer struct {
	pluginv1.UnimplementedMetadataProviderServer
	runtime *runtimeServer
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *runtimeServer) Configure(_ context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = make(map[string]map[string]any, len(req.GetConfig()))
	for _, entry := range req.GetConfig() {
		if entry == nil {
			continue
		}
		s.config[entry.GetKey()] = entry.GetValue().AsMap()
	}
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *metadataServer) Search(context.Context, *pluginv1.SearchMetadataRequest) (*pluginv1.SearchMetadataResponse, error) {
	title := "Example Title"
	if s.runtime != nil {
		s.runtime.mu.RLock()
		if searchConfig := s.runtime.config["search"]; searchConfig != nil {
			if configuredTitle, ok := searchConfig["title"].(string); ok && configuredTitle != "" {
				title = configuredTitle
			}
		}
		s.runtime.mu.RUnlock()
	}
	return &pluginv1.SearchMetadataResponse{
		Results: []*pluginv1.ProviderSearchResult{
			{
				ProviderId: "example-1",
				ItemType:   "movie",
				Title:      title,
			},
		},
	}, nil
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	runtimeServer := &runtimeServer{manifest: manifest}

	runtime.Serve(runtime.ServeConfig{
		Servers: runtime.CapabilityServers{
			Runtime:          runtimeServer,
			MetadataProvider: &metadataServer{runtime: runtimeServer},
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
