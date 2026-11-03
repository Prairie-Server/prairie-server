package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/prairie-server/prairie-server/internal/historyimport"
	"github.com/prairie-server/prairie-server/internal/netguard"
)

func TestWebhookManagementErrorExplainsRefusedServerAddress(t *testing.T) {
	t.Parallel()

	for cause, message := range map[error]string{
		netguard.ErrPrivateDestination: historyimport.PrivateAddressMessage,
		netguard.ErrBlockedDestination: historyimport.BlockedAddressMessage,
	} {
		var out *APIError
		if !errors.As(webhookManagementError(fmt.Errorf("creating connection: %w", cause)), &out) {
			t.Fatal("webhookManagementError did not return an APIError")
		}
		if out.Status != http.StatusBadRequest || out.Code != policyErrorBadRequest || out.Message != message {
			t.Fatalf("webhookManagementError(%v) = %+v", cause, out)
		}
	}
}
