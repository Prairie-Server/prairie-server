package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "manifest" {
		fmt.Fprintln(os.Stderr, "unsupported command")
		os.Exit(1)
	}

	executablePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		panic(err)
	}
	checksum := sha256.Sum256(binaryData)

	manifest := &pluginv1.PluginManifest{
		PluginId:          "prairie.binary",
		Version:           "1.0.0",
		Checksum:          hex.EncodeToString(checksum[:]),
		PrairieApiVersion: "v1",
		SupportedPlatforms: []*pluginv1.SupportedPlatform{
			{Os: runtime.GOOS, Arch: runtime.GOARCH},
		},
		Capabilities: []*pluginv1.CapabilityDescriptor{
			{
				Type:        "metadata_provider.v1",
				Id:          "binary",
				DisplayName: "Binary Manifest",
				Description: "Capability data sourced from the downloaded binary.",
			},
		},
		GlobalConfigSchema: []*pluginv1.ConfigSchema{
			{
				Key:         "binary_config",
				Title:       "Binary Config",
				Description: "Only present in the binary manifest output.",
				JsonSchema:  `{"type":"object","properties":{"token":{"type":"string"}}}`,
			},
		},
	}

	data, err := protojson.Marshal(manifest)
	if err != nil {
		panic(err)
	}
	fmt.Print(string(data))
}
