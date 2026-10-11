package intromarkers

import (
	"regexp"
	"strings"

	"github.com/prairie-server/prairie-server/internal/models"
)

var introChapterPattern = regexp.MustCompile(`(?i)(^|\s)(intro|introduction|opening)(\s|:|$)`)
var generatedChapterPattern = regexp.MustCompile(`(?i)^chapter\s*\d+$`)

func DetectChapterIntro(chapters []models.MediaChapter) (Segment, bool) {
	for i, chapter := range chapters {
		title := strings.TrimSpace(chapter.Title)
		if !isIntroChapterTitle(title) {
			continue
		}
		if i > 0 && isIntroChapterTitle(strings.TrimSpace(chapters[i-1].Title)) {
			continue
		}
		if i+1 < len(chapters) && isIntroChapterTitle(strings.TrimSpace(chapters[i+1].Title)) {
			continue
		}

		end := chapter.EndSeconds
		if i+1 < len(chapters) && chapters[i+1].StartSeconds > chapter.StartSeconds {
			end = chapters[i+1].StartSeconds
		}
		duration := end - chapter.StartSeconds
		if duration < 10 || duration > 180 {
			continue
		}
		return Segment{
			Start:      chapter.StartSeconds,
			End:        end,
			Confidence: 0.95,
			Algorithm:  ChapterAlgorithm,
		}, true
	}
	return Segment{}, false
}

func isIntroChapterTitle(title string) bool {
	if title == "" || generatedChapterPattern.MatchString(title) {
		return false
	}
	if introChapterPattern.MatchString(title) {
		return true
	}
	return isExplicitOPChapterTitle(title)
}

func isExplicitOPChapterTitle(title string) bool {
	if !strings.HasPrefix(title, "OP") {
		return false
	}
	if len(title) == len("OP") {
		return true
	}
	switch next := title[len("OP")]; {
	case next >= '0' && next <= '9':
		return true
	case next == ' ' || next == ':' || next == '-':
		return strings.TrimSpace(title[len("OP")+1:]) != ""
	default:
		return false
	}
}
