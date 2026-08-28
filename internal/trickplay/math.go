package trickplay

import (
	"math"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

const (
	DefaultIntervalSeconds = 10.0
	DefaultTileWidth       = 320
	DefaultTileColumns     = 10
	DefaultTileRows        = 10
)

// TileIndex returns the zero-based tile covering the given playback time.
func TileIndex(seconds, interval float64) int {
	if interval <= 0 {
		interval = DefaultIntervalSeconds
	}
	if seconds < 0 {
		seconds = 0
	}
	return int(math.Floor(seconds / interval))
}

// SheetIndex returns the zero-based sheet containing the given tile.
func SheetIndex(tileIndex, tilesPerSheet int) int {
	if tilesPerSheet <= 0 {
		tilesPerSheet = DefaultTileColumns * DefaultTileRows
	}
	if tileIndex < 0 {
		tileIndex = 0
	}
	return tileIndex / tilesPerSheet
}

// TilePosition returns the column and row of a tile within its sheet.
func TilePosition(tileIndex, columns, rows int) (col, row int) {
	if columns <= 0 {
		columns = DefaultTileColumns
	}
	if rows <= 0 {
		rows = DefaultTileRows
	}
	tilesPerSheet := columns * rows
	local := tileIndex % tilesPerSheet
	if local < 0 {
		local = 0
	}
	col = local % columns
	row = local / columns
	return col, row
}

// TilesPerSheet returns columns*rows with defaults applied.
func TilesPerSheet(columns, rows int) int {
	if columns <= 0 {
		columns = DefaultTileColumns
	}
	if rows <= 0 {
		rows = DefaultTileRows
	}
	return columns * rows
}

// ExpectedThumbnailCount returns how many interval tiles a duration needs.
func ExpectedThumbnailCount(durationSeconds int, interval float64) int {
	if durationSeconds <= 0 {
		return 0
	}
	if interval <= 0 {
		interval = DefaultIntervalSeconds
	}
	return int(math.Ceil(float64(durationSeconds) / interval))
}

// ExpectedSheetCount returns how many sheets are needed for thumbnailCount tiles.
func ExpectedSheetCount(thumbnailCount, tilesPerSheet int) int {
	if thumbnailCount <= 0 {
		return 0
	}
	if tilesPerSheet <= 0 {
		tilesPerSheet = DefaultTileColumns * DefaultTileRows
	}
	return int(math.Ceil(float64(thumbnailCount) / float64(tilesPerSheet)))
}

// SheetStartSeconds returns the seek offset for generating sheetIndex.
func SheetStartSeconds(sheetIndex int, interval float64, tilesPerSheet int) float64 {
	if interval <= 0 {
		interval = DefaultIntervalSeconds
	}
	if tilesPerSheet <= 0 {
		tilesPerSheet = DefaultTileColumns * DefaultTileRows
	}
	if sheetIndex < 0 {
		sheetIndex = 0
	}
	return float64(sheetIndex) * float64(tilesPerSheet) * interval
}

// IsIncomplete reports whether stored trickplay metadata is missing or stale
// relative to the file's current duration.
func IsIncomplete(tp *models.MediaTrickplay, durationSeconds int) bool {
	if tp == nil {
		return true
	}
	if tp.ThumbnailCount <= 0 {
		return true
	}
	if tp.DurationSeconds != durationSeconds {
		return true
	}
	interval := tp.IntervalSeconds
	if interval <= 0 {
		interval = DefaultIntervalSeconds
	}
	expectedThumbs := ExpectedThumbnailCount(durationSeconds, interval)
	if expectedThumbs <= 0 {
		return true
	}
	if tp.ThumbnailCount < expectedThumbs {
		return true
	}
	tilesPerSheet := TilesPerSheet(tp.TileColumns, tp.TileRows)
	expectedSheets := ExpectedSheetCount(expectedThumbs, tilesPerSheet)
	if expectedSheets <= 0 {
		return true
	}
	present := make(map[int]struct{}, len(tp.Sheets))
	for _, sheet := range tp.Sheets {
		if sheet.Path == "" {
			continue
		}
		present[sheet.Index] = struct{}{}
	}
	for i := 0; i < expectedSheets; i++ {
		if _, ok := present[i]; !ok {
			return true
		}
	}
	return false
}

// IsRetryEligible reports whether a failed trickplay state may be retried now.
func IsRetryEligible(tp *models.MediaTrickplay, now time.Time) bool {
	if tp == nil {
		return true
	}
	if tp.RetryAfter == nil {
		return true
	}
	return !tp.RetryAfter.After(now)
}
