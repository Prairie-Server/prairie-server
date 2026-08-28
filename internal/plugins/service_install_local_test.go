package plugins

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestServiceInstallLocalReplacesExistingInstallation(t *testing.T) {
	ctx := context.Background()

	oldDir := t.TempDir()
	oldPath := filepath.Join(oldDir, "plugin")
	if err := os.WriteFile(oldPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", oldPath, err)
	}

	archivePath := filepath.Join(t.TempDir(), "prairie-metadb.zip")
	manifest := testPluginManifest(t, "prairie.metadb", "0.0.36")
	writePluginArchive(t, archivePath, manifest)

	events := []string{}
	store := newFakeServiceInstallationStore(&Installation{
		ID:          7,
		PluginID:    "prairie.metadb",
		Version:     "0.0.34",
		InstallPath: oldPath,
		Enabled:     true,
	})
	store.events = &events

	host := &fakeServiceHost{events: &events}
	service := &Service{
		installations: store,
		installer:     NewInstaller(store, InstallerOptions{BaseDir: t.TempDir()}),
		host:          host,
	}

	result, err := service.InstallLocal(ctx, InstallArchiveRequest{ArchivePath: archivePath})
	if err != nil {
		t.Fatalf("InstallLocal() returned error: %v", err)
	}

	if result.Installation.ID != 7 {
		t.Fatalf("result installation id = %d, want 7", result.Installation.ID)
	}
	if result.Installation.Version != "0.0.36" {
		t.Fatalf("result installation version = %q, want 0.0.36", result.Installation.Version)
	}
	if result.Installation.InstallPath == oldPath {
		t.Fatal("expected InstallLocal() replacement to move the installation to a new path")
	}
	if len(host.stopped) != 1 || host.stopped[0] != 7 {
		t.Fatalf("stopped installations = %#v, want [7]", host.stopped)
	}
	if len(store.createInputs) != 0 {
		t.Fatalf("Create() called %d times, want 0", len(store.createInputs))
	}
	if len(store.saveArchiveIDs) != 1 || store.saveArchiveIDs[0] != 7 {
		t.Fatalf("SaveArchive() ids = %#v, want [7]", store.saveArchiveIDs)
	}
	if len(store.updateIDs) != 1 || store.updateIDs[0] != 7 {
		t.Fatalf("Update() ids = %#v, want [7]", store.updateIDs)
	}
	if got := store.byID[7]; got == nil || got.Version != "0.0.36" {
		t.Fatalf("stored installation version = %#v, want updated version 0.0.36", got)
	}

	wantEvents := []string{"stop", "save_archive", "update"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
}
