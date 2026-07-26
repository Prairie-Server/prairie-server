package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicmanifest "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/manifest"
	"github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/runtime"
)

type runtimeServer struct {
	pluginv1.UnimplementedRuntimeServer
	manifest *pluginv1.PluginManifest
}

type routesServer struct {
	pluginv1.UnimplementedHttpRoutesServer
}

//go:embed manifest.json
var manifestJSON []byte

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *routesServer) Handle(context.Context, *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return &pluginv1.HandleHTTPResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte("fakeroutes"),
	}, nil
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	runtime.Serve(runtime.ServeConfig{
		Servers: runtime.CapabilityServers{
			Runtime:    &runtimeServer{manifest: manifest},
			HttpRoutes: &routesServer{},
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])

	return manifest, nil
}
