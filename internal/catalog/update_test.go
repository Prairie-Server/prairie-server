package catalog

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestTitleLockedForAdminUpdate(t *testing.T) {
	item := &models.MediaItem{LockedFields: []int{fieldNameLocked}}

	t.Run("uses request locks when provided", func(t *testing.T) {
		unlocked := []int{1}
		upd := &MetadataUpdate{LockedFields: &unlocked}
		if titleLockedForAdminUpdate(item, upd) {
			t.Fatal("expected title unlocked from request locked_fields")
		}
	})

	t.Run("falls back to stored locks when request omits locked_fields", func(t *testing.T) {
		upd := &MetadataUpdate{}
		if !titleLockedForAdminUpdate(item, upd) {
			t.Fatal("expected title locked from stored locked_fields")
		}
	})

	t.Run("request can lock title when stored is unlocked", func(t *testing.T) {
		unlockedItem := &models.MediaItem{LockedFields: []int{}}
		locked := []int{fieldNameLocked}
		upd := &MetadataUpdate{LockedFields: &locked}
		if !titleLockedForAdminUpdate(unlockedItem, upd) {
			t.Fatal("expected title locked from request locked_fields")
		}
	})
}
