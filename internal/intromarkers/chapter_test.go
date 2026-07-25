package intromarkers

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/models"
)

func TestDetectChapterIntroUsesNextChapterBoundary(t *testing.T) {
	segment, ok := DetectChapterIntro([]models.MediaChapter{
		{Index: 0, Title: "Cold Open", StartSeconds: 0, EndSeconds: 60},
		{Index: 1, Title: "Opening", StartSeconds: 60, EndSeconds: 120},
		{Index: 2, Title: "Part 1", StartSeconds: 95, EndSeconds: 900},
	})
	if !ok {
		t.Fatal("expected intro chapter")
	}
	if segment.Start != 60 || segment.End != 95 {
		t.Fatalf("unexpected segment %.1f-%.1f", segment.Start, segment.End)
	}
	if segment.Confidence != 0.95 || segment.Algorithm != ChapterAlgorithm {
		t.Fatalf("unexpected metadata: %#v", segment)
	}
}

func TestDetectChapterIntroRejectsGeneratedAndAdjacentMatches(t *testing.T) {
	if _, ok := DetectChapterIntro([]models.MediaChapter{
		{Index: 0, Title: "Chapter 01", StartSeconds: 0, EndSeconds: 30},
		{Index: 1, Title: "Part 1", StartSeconds: 30, EndSeconds: 600},
	}); ok {
		t.Fatal("generated chapter title should not match")
	}
	if _, ok := DetectChapterIntro([]models.MediaChapter{
		{Index: 0, Title: "Intro", StartSeconds: 0, EndSeconds: 30},
		{Index: 1, Title: "Opening", StartSeconds: 30, EndSeconds: 90},
		{Index: 2, Title: "Part 1", StartSeconds: 90, EndSeconds: 600},
	}); ok {
		t.Fatal("adjacent intro-like chapters should not match")
	}
}

func TestDetectChapterIntroRejectsAmbiguousOpusTitles(t *testing.T) {
	for _, title := range []string{"op", "Op", "Op. 5", "Op 5"} {
		t.Run(title, func(t *testing.T) {
			if _, ok := DetectChapterIntro([]models.MediaChapter{
				{Index: 0, Title: title, StartSeconds: 0, EndSeconds: 45},
				{Index: 1, Title: "Movement I", StartSeconds: 45, EndSeconds: 600},
			}); ok {
				t.Fatalf("ambiguous title %q should not match", title)
			}
		})
	}
}

func TestDetectChapterIntroAcceptsExplicitOPTitles(t *testing.T) {
	for _, title := range []string{"OP", "OP1", "OP 1", "OP: Opening", "OP - Opening"} {
		t.Run(title, func(t *testing.T) {
			segment, ok := DetectChapterIntro([]models.MediaChapter{
				{Index: 0, Title: title, StartSeconds: 0, EndSeconds: 45},
				{Index: 1, Title: "Part 1", StartSeconds: 45, EndSeconds: 600},
			})
			if !ok {
				t.Fatalf("explicit title %q should match", title)
			}
			if segment.Start != 0 || segment.End != 45 {
				t.Fatalf("unexpected segment %.1f-%.1f", segment.Start, segment.End)
			}
		})
	}
}
