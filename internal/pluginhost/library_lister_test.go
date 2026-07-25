package pluginhost_test

import (
	"context"
	"testing"

	"github.com/prairie-server/prairie-server/internal/pluginhost"
)

func TestLibraryLister_AllLibraries(t *testing.T) {
	src := pluginhost.LibraryDataSourceFunc(
		func(_ context.Context, userID string) ([]pluginhost.LibraryRecord, error) {
			if userID != "" {
				t.Errorf("expected empty userID for admin scope, got %q", userID)
			}
			return []pluginhost.LibraryRecord{{ID: "1", Name: "Movies", MediaType: "movie"}}, nil
		},
	)
	a := pluginhost.NewLibraryLister(src)

	got, err := a.ListLibraries(context.Background(), "")
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("got %+v", got)
	}
}

func TestLibraryLister_UserScope(t *testing.T) {
	called := false
	src := pluginhost.LibraryDataSourceFunc(
		func(_ context.Context, userID string) ([]pluginhost.LibraryRecord, error) {
			called = true
			if userID != "u-1" {
				t.Errorf("userID = %q, want %q", userID, "u-1")
			}
			return nil, nil
		},
	)
	if _, err := pluginhost.NewLibraryLister(src).ListLibraries(context.Background(), "u-1"); err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if !called {
		t.Error("data source func was not called")
	}
}
