package downloads

import (
	"testing"

	"github.com/prairie-server/prairie-server/internal/config"
)

func TestProxyDeliveryAllowedPreservesAggregateBandwidthLimits(t *testing.T) {
	if !proxyDeliveryAllowed(config.DownloadConfig{}) {
		t.Fatal("uncapped downloads should be proxy eligible")
	}
	if proxyDeliveryAllowed(config.DownloadConfig{ServerBandwidthBPS: 1}) {
		t.Fatal("server-capped downloads must stay API-local")
	}
	if proxyDeliveryAllowed(config.DownloadConfig{UserBandwidthBPS: 1}) {
		t.Fatal("user-capped downloads must stay API-local")
	}
}
