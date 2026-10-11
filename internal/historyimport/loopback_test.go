package historyimport

import (
	"context"

	"github.com/prairie-server/prairie-server/internal/netguard"
)

// trustLoopback marks ctx as trusted with the local network. Test upstream
// servers listen on loopback, which the outbound guard lets only trusted
// requests reach.
func trustLoopback(ctx context.Context) context.Context {
	return netguard.WithPrivateAccess(ctx)
}
