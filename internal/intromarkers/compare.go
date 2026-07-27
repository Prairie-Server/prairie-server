package intromarkers

import (
	"context"
)

type fingerprintInput struct {
	Candidate Candidate
	Points    []uint32
}

func CompareFingerprints(inputs []fingerprintInput, cfg Config) map[int]Segment {
	cfg = cfg.normalized()
	best := map[int]Segment{}
	confirmations := map[int]int{}

	if len(inputs) == 0 {
		return best
	}

	p, err := getCompareProcessor()
	if err != nil {
		return best
	}
	matches, err := p.compare(context.Background(), inputs, cfg)
	if err != nil || len(matches) == 0 {
		return best
	}

	for _, match := range matches {
		if match.LeftIndex < 0 || match.LeftIndex >= len(inputs) ||
			match.RightIndex < 0 || match.RightIndex >= len(inputs) {
			continue
		}
		leftInput := inputs[match.LeftIndex]
		rightInput := inputs[match.RightIndex]

		leftSeg := adjustSegment(Segment{Start: match.Left.Start, End: match.Left.End}, leftInput.Candidate)
		rightSeg := adjustSegment(Segment{Start: match.Right.Start, End: match.Right.End}, rightInput.Candidate)
		if validAdjustedSegment(leftSeg) {
			recordBest(best, confirmations, leftInput.Candidate.FileID, leftSeg)
		}
		if validAdjustedSegment(rightSeg) {
			recordBest(best, confirmations, rightInput.Candidate.FileID, rightSeg)
		}
	}

	for fileID, segment := range best {
		confidence := 0.65
		if segment.End-segment.Start >= 30 {
			confidence += 0.10
		}
		if confirmations[fileID] >= 2 {
			confidence += 0.10
		}
		if segment.Start == 0 {
			confidence += 0.05
		}
		if confidence > 0.90 {
			confidence = 0.90
		}
		segment.Confidence = confidence
		segment.Algorithm = ChromaprintAlgorithm
		best[fileID] = segment
	}

	return best
}

func adjustSegment(segment Segment, candidate Candidate) Segment {
	if segment.Start <= 5 {
		segment.Start = 0
	}
	for _, chapter := range candidate.Chapters {
		segment.Start = snapBoundary(segment.Start, chapter.StartSeconds)
		segment.End = snapBoundary(segment.End, chapter.StartSeconds)
		segment.End = snapBoundary(segment.End, chapter.EndSeconds)
	}
	if segment.Start < 0 {
		segment.Start = 0
	}
	if candidate.DurationSeconds > 0 && segment.End > candidate.DurationSeconds {
		segment.End = candidate.DurationSeconds
	}
	return segment
}

func snapBoundary(value, boundary float64) float64 {
	delta := boundary - value
	if delta >= -5 && delta <= 2 {
		return boundary
	}
	return value
}

func validAdjustedSegment(segment Segment) bool {
	duration := segment.End - segment.Start
	return segment.Start >= 0 && segment.End > segment.Start && duration >= 10 && duration <= 180
}

func recordBest(best map[int]Segment, confirmations map[int]int, fileID int, segment Segment) {
	confirmations[fileID]++
	if current, ok := best[fileID]; !ok || segment.End-segment.Start > current.End-current.Start {
		best[fileID] = segment
	}
}
