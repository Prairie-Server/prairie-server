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

type authServer struct {
	pluginv1.UnimplementedAuthProviderServer
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *authServer) Authenticate(context.Context, *pluginv1.AuthenticateRequest) (*pluginv1.AuthenticateResponse, error) {
	return &pluginv1.AuthenticateResponse{
		ExternalSubject: "fakeauth-user",
		DisplayName:     "Fake Auth User",
		Email:           "fakeauth@example.com",
	}, nil
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	runtime.Serve(runtime.ServeConfig{
		Servers: runtime.CapabilityServers{
			Runtime:      &runtimeServer{manifest: manifest},
			AuthProvider: &authServer{},
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
