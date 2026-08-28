package sections

import (
	"context"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/sections/recipes"
)

func TestInstallRecipeDelegateRegisters(t *testing.T) {
	InstallRecipeDelegate(nil) // tolerates nil fetcher: delegate set to a function that returns ErrNoFetcher

	rec, ok := recipes.Get("recently_added")
	if !ok {
		t.Fatal("recently_added not registered")
	}

	rc := recipes.ResolverContext{Ctx: context.Background(), Now: time.Now()}
	_, err := rec.Resolve(rc)
	if err == nil {
		t.Fatal("expected ErrNoFetcher")
	}
}
