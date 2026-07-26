package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicmanifest "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/manifest"
	"github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/runtime"
)

type runtimeServer struct {
	pluginv1.UnimplementedRuntimeServer
	manifest *pluginv1.PluginManifest
}

type taskServer struct {
	pluginv1.UnimplementedScheduledTaskServer
}

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *taskServer) Run(context.Context, *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
	output, _ := structpb.NewStruct(map[string]any{"status": "ok"})
	return &pluginv1.RunScheduledTaskResponse{Output: output}, nil
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	runtime.Serve(runtime.ServeConfig{
		Servers: runtime.CapabilityServers{
			Runtime:       &runtimeServer{manifest: manifest},
			ScheduledTask: &taskServer{},
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
