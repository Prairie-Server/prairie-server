package trickplay

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestTileIndexMath(t *testing.T) {
	if got := TileIndex(0, 10); got != 0 {
		t.Fatalf("TileIndex(0) = %d, want 0", got)
	}
	if got := TileIndex(9.9, 10); got != 0 {
		t.Fatalf("TileIndex(9.9) = %d, want 0", got)
	}
	if got := TileIndex(10, 10); got != 1 {
		t.Fatalf("TileIndex(10) = %d, want 1", got)
	}
	if got := TileIndex(1005, 10); got != 100 {
		t.Fatalf("TileIndex(1005) = %d, want 100", got)
	}
}

func TestSheetIndexAndTilePosition(t *testing.T) {
	if got := SheetIndex(0, 100); got != 0 {
		t.Fatalf("SheetIndex(0) = %d, want 0", got)
	}
	if got := SheetIndex(99, 100); got != 0 {
		t.Fatalf("SheetIndex(99) = %d, want 0", got)
	}
	if got := SheetIndex(100, 100); got != 1 {
		t.Fatalf("SheetIndex(100) = %d, want 1", got)
	}

	col, row := TilePosition(0, 10, 10)
	if col != 0 || row != 0 {
		t.Fatalf("TilePosition(0) = (%d,%d), want (0,0)", col, row)
	}
	col, row = TilePosition(11, 10, 10)
	if col != 1 || row != 1 {
		t.Fatalf("TilePosition(11) = (%d,%d), want (1,1)", col, row)
	}
	col, row = TilePosition(105, 10, 10)
	if col != 5 || row != 0 {
		t.Fatalf("TilePosition(105) = (%d,%d), want (5,0)", col, row)
	}
}

func TestIsIncomplete(t *testing.T) {
	if !IsIncomplete(nil, 100) {
		t.Fatal("nil trickplay should be incomplete")
	}
	if !IsIncomplete(&models.MediaTrickplay{ThumbnailCount: 0, DurationSeconds: 100}, 100) {
		t.Fatal("zero thumbnail_count should be incomplete")
	}
	if !IsIncomplete(&models.MediaTrickplay{
		IntervalSeconds: 10,
		TileColumns:     10,
		TileRows:        10,
		ThumbnailCount:  10,
		DurationSeconds: 50,
		Sheets:          []models.MediaTrickplaySheet{{Index: 0, Path: "a"}},
	}, 100) {
		t.Fatal("duration mismatch should be incomplete")
	}

	complete := &models.MediaTrickplay{
		IntervalSeconds: 10,
		TileColumns:     10,
		TileRows:        10,
		ThumbnailCount:  10,
		DurationSeconds: 100,
		Sheets:          []models.MediaTrickplaySheet{{Index: 0, Path: "a.webp"}},
	}
	if IsIncomplete(complete, 100) {
		t.Fatal("complete trickplay reported incomplete")
	}

	missingSheet := &models.MediaTrickplay{
		IntervalSeconds: 10,
		TileColumns:     10,
		TileRows:        10,
		ThumbnailCount:  150,
		DurationSeconds: 1500,
		Sheets:          []models.MediaTrickplaySheet{{Index: 0, Path: "a.webp"}},
	}
	if !IsIncomplete(missingSheet, 1500) {
		t.Fatal("missing second sheet should be incomplete")
	}
}

func TestSelectSheetIndicesPrioritizesTarget(t *testing.T) {
	missing := []int{0, 1, 2, 3, 4}
	target := 1050.0 // tile 105 → sheet 1
	got := selectSheetIndices(missing, &target, 10, 100, true, 2)
	if len(got) != 2 || got[0] != 1 {
		t.Fatalf("selectSheetIndices() = %#v, want sheet 1 first", got)
	}
}

type testFileRepo struct {
	file          *models.MediaFile
	updateCalls   int
	lastTrickplay *models.MediaTrickplay
}

func (r *testFileRepo) GetByID(context.Context, int) (*models.MediaFile, error) {
	if r.file == nil {
		return nil, nil
	}
	cp := *r.file
	if r.file.Trickplay != nil {
		tp := *r.file.Trickplay
		tp.Sheets = append([]models.MediaTrickplaySheet(nil), r.file.Trickplay.Sheets...)
		cp.Trickplay = &tp
	}
	return &cp, nil
}

func (r *testFileRepo) ListMissingTrickplay(context.Context, int) ([]*models.MediaFile, error) {
	return nil, nil
}

func (r *testFileRepo) UpdateTrickplayState(_ context.Context, _ int, trickplay *models.MediaTrickplay) (*models.MediaFile, error) {
	r.updateCalls++
	if trickplay != nil {
		cp := *trickplay
		cp.Sheets = append([]models.MediaTrickplaySheet(nil), trickplay.Sheets...)
		r.lastTrickplay = &cp
		if r.file != nil {
			r.file.Trickplay = &cp
		}
	}
	return r.GetByID(context.Background(), 0)
}

type testFolderRepo struct {
	folder *models.MediaFolder
}

func (r *testFolderRepo) GetByID(context.Context, int) (*models.MediaFolder, error) {
	return r.folder, nil
}

type testStore struct{}

func (testStore) PutObject(context.Context, string, string, []byte) error { return nil }
func (testStore) Bucket() string                                          { return "test" }

func TestProcessRequestGeneratesPrioritySheet(t *testing.T) {
	fileRepo := &testFileRepo{
		file: &models.MediaFile{
			ID:            7,
			MediaFolderID: 1,
			FilePath:      "/media/a.mkv",
			Duration:      1500,
		},
	}
	service := NewService(fileRepo, &testFolderRepo{
		folder: &models.MediaFolder{ID: 1, Enabled: true, TrickplayEnabled: true},
	}, nil, testStore{}, "ffmpeg", 1)
	service.extractSheetFunc = func(context.Context, *models.MediaFile, float64, bool) ([]byte, string, error) {
		return []byte("jpeg"), "", nil
	}
	service.uploadSheetFunc = func(_ context.Context, fileID, sheetIndex int, _ []byte) (string, error) {
		return fmt.Sprintf("trickplay/%d/%d.webp", fileID, sheetIndex), nil
	}
	service.clock = func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }

	target := 1050.0
	requeue, err := service.processRequest(context.Background(), TrickplayRequest{FileID: 7, TargetSeconds: &target}, true)
	if err != nil {
		t.Fatalf("processRequest() error = %v", err)
	}
	if !requeue {
		t.Fatal("expected requeue for remaining sheets")
	}
	if fileRepo.updateCalls != 1 {
		t.Fatalf("updateCalls = %d, want 1", fileRepo.updateCalls)
	}
	if fileRepo.lastTrickplay == nil || len(fileRepo.lastTrickplay.Sheets) != 1 {
		t.Fatalf("sheets = %#v, want one sheet", fileRepo.lastTrickplay)
	}
	if fileRepo.lastTrickplay.Sheets[0].Index != 1 {
		t.Fatalf("generated sheet index = %d, want 1", fileRepo.lastTrickplay.Sheets[0].Index)
	}
}

func TestBuildSheetExtractArgsIncludesTonemap(t *testing.T) {
	args := buildSheetExtractArgs("/media/a.mkv", 1000, 10, 320, 10, 10, true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "tonemap=bt2390") {
		t.Fatalf("missing tonemap in args: %v", args)
	}
	if !strings.Contains(joined, "fps=1/10,scale=320:-2,tile=10x10") {
		t.Fatalf("missing tile filter in args: %v", args)
	}
}
