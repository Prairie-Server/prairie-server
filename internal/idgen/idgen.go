// Package idgen provides unique ID generation for Prairie entities.
// All content IDs (media items, seasons, episodes) are generated locally
// using Sonyflake — a distributed unique ID generator that produces
// time-sorted 64-bit integers encoded as decimal strings.
package idgen

import (
	"fmt"
	"strconv"
	"time"

	"github.com/sony/sonyflake"
)

// epoch is the Sonyflake epoch: 2024-01-01 UTC.
// Sonyflake measures elapsed time from this point.
var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

var sf *sonyflake.Sonyflake

func init() {
	sf = sonyflake.NewSonyflake(sonyflake.Settings{
		StartTime: epoch,
	})
	if sf == nil {
		panic("idgen: failed to initialize sonyflake")
	}
}

// NextID returns a new unique Sonyflake ID as a decimal string.
func NextID() (string, error) {
	id, err := sf.NextID()
	if err != nil {
		return "", fmt.Errorf("idgen: %w", err)
	}
	return strconv.FormatUint(id, 10), nil
}
