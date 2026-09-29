package trickplay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"os/exec"
	"path/filepath"
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
	args := buildSheetExtractArgs("/media/a.mkv", 1000, 10, 320, 10, 10, true, true)
	joined := strings.Join(args, " ")
	// Plain tonemap has no bt2390; only jellyfin-ffmpeg's tonemapx does.
	if !strings.Contains(joined, "fps=1/10,scale=320:-2,tonemapx=tonemap=bt2390,") {
		t.Fatalf("missing tonemapx after fps/scale in args: %v", args)
	}
	if !strings.HasSuffix(strings.Split(joined, " -vf ")[1], "format=yuv420p,tile=10x10 -frames:v 1 -f image2pipe -vcodec mjpeg -") {
		t.Fatalf("tile must follow tone mapping: %v", args)
	}
	if !strings.Contains(joined, "-skip_frame nokey -ss") {
		t.Fatalf("missing keyframe-only decode in args: %v", args)
	}
}

func TestBuildSheetExtractArgsSDRSkipsTonemap(t *testing.T) {
	args := buildSheetExtractArgs("/media/a.mkv", 0, 10, 320, 10, 10, false, false)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "tonemap") {
		t.Fatalf("SDR args must not tone map: %v", args)
	}
	if !strings.Contains(joined, "-vf fps=1/10,scale=320:-2,tile=10x10 ") {
		t.Fatalf("missing tile filter in args: %v", args)
	}
	if strings.Contains(joined, "-skip_frame") {
		t.Fatalf("full decode must not skip frames: %v", args)
	}
}

func TestKeyframesDenseEnough(t *testing.T) {
	tests := []struct {
		name      string
		keyframes []float64
		want      bool
	}{
		{"blu-ray 1s GOP", []float64{600, 601, 602.1, 603, 604}, true},
		{"gap exactly half the interval", []float64{600, 605, 610}, true},
		{"WEB 10s GOP", []float64{600, 610.1, 620.2}, false},
		{"one long gap", []float64{600, 602, 614, 616}, false},
		{"single keyframe", []float64{600}, false},
		{"none", nil, false},
	}
	for _, tt := range tests {
		if got := keyframesDenseEnough(tt.keyframes, 10); got != tt.want {
			t.Errorf("%s: keyframesDenseEnough = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExtractSheetDecodesKeyframesOnlyWhenTheyAreDense(t *testing.T) {
	tests := []struct {
		name      string
		keyframes []float64
		probeErr  error
		wantSkip  bool
	}{
		{"dense keyframes", []float64{600, 601, 602, 603}, nil, true},
		{"sparse keyframes", []float64{600, 610, 620}, nil, false},
		{"probe failure", nil, errors.New("ffprobe failed"), false},
	}
	for _, tt := range tests {
		var gotArgs []string
		_, _, err := ExtractSheet(context.Background(), SheetExtractOptions{
			InputPath:  "/media/a.mkv",
			SheetStart: 600,
			RunFunc: func(_ context.Context, _ string, args []string) ([]byte, error) {
				gotArgs = args
				return []byte("jpeg"), nil
			},
			ProbeKeyframes: func(context.Context, string, string, float64, float64) ([]float64, error) {
				return tt.keyframes, tt.probeErr
			},
		})
		if err != nil {
			t.Fatalf("%s: ExtractSheet: %v", tt.name, err)
		}
		if got := strings.Contains(strings.Join(gotArgs, " "), "-skip_frame nokey"); got != tt.wantSkip {
			t.Errorf("%s: keyframe-only decode = %v, want %v (args %v)", tt.name, got, tt.wantSkip, gotArgs)
		}
	}
}

// The probe must judge the stream the extract tiles, and neither may pick an
// attached cover picture (v:0 would).
func TestProbeAndExtractReadTheSameVideoStream(t *testing.T) {
	probe := strings.Join(buildKeyframeProbeArgs("/media/a.mkv", 600, 60), " ")
	extract := strings.Join(buildSheetExtractArgs("/media/a.mkv", 600, 10, 320, 10, 10, false, false), " ")
	if !strings.Contains(probe, "-select_streams V:0 ") {
		t.Fatalf("probe does not select the first non-picture video stream: %s", probe)
	}
	if !strings.Contains(extract, "-i /media/a.mkv -map 0:V:0 ") {
		t.Fatalf("extract does not map the probed stream: %s", extract)
	}
}

// With two video streams of different GOP spacing, the probe reports the
// first stream's keyframes and the extract tiles that same stream.
func TestKeyframeProbeFollowsTheExtractedStream(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	// Stream 0: 16:9, keyframe every 1s. Stream 1: square, keyframe every 40s.
	// The aspect ratio tells the tiled stream apart in the output sheet.
	input := filepath.Join(t.TempDir(), "two-streams.mkv")
	gen := exec.Command(ffmpeg, "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=160x90:rate=10:duration=40",
		"-f", "lavfi", "-i", "testsrc=size=320x320:rate=10:duration=40",
		"-map", "0:v", "-map", "1:v",
		"-c:v", "mpeg4", "-g:v:0", "10", "-g:v:1", "400",
		// Flag the second stream default so ffmpeg's own pick (default
		// disposition, then resolution) differs from the probed stream.
		"-disposition:v:0", "0", "-disposition:v:1", "default",
		input)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, out)
	}

	keyframes, err := probeKeyframeTimes(context.Background(), ffprobe, input, 0, 30)
	if err != nil {
		t.Fatalf("probeKeyframeTimes: %v", err)
	}
	if len(keyframes) < 20 || !keyframesDenseEnough(keyframes, 10) {
		t.Fatalf("probe did not read the dense first stream: %v", keyframes)
	}

	sheet, err := runFFmpegSheetExtract(context.Background(), ffmpeg,
		buildSheetExtractArgs(input, 0, 10, 160, 2, 2, false, true))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(sheet))
	if err != nil {
		t.Fatalf("decode sheet: %v", err)
	}
	if cfg.Width != 320 || cfg.Height != 180 {
		t.Fatalf("sheet is %dx%d, want 320x180 tiled from the first stream", cfg.Width, cfg.Height)
	}
}
