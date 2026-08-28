package models

import "time"

// ArtworkAVIFBackfillJob is a durable deferred AVIF sibling encode/upload.
type ArtworkAVIFBackfillJob struct {
	ID            int64
	OriginalPath  string
	ImageType     string
	Status        string
	AttemptCount  int
	NextAttemptAt time.Time
	LockedAt      *time.Time
	LockedBy      string
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}
