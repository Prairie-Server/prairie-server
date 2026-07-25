package pluginhost_test

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/pluginhost"
)

func TestConfig_AcceptsEventPublisherAndLibraryLister(t *testing.T) {
	// Compile-time assertion that pluginhost.Config has these fields.
	_ = pluginhost.Config{
		EventPublisher: nil,
		LibraryLister:  nil,
	}
}
