package handlers

import (
	"context"

	"github.com/prairie-server/prairie-server/internal/notifications"
)

func (h *NotificationsHandler) OrderedAndroidPushAvailable() bool {
	return h.pushDevices().OrderedAndroidAvailable()
}
func (h *NotificationsHandler) ApplyAndroidPush(ctx context.Context, cmd notifications.AndroidPushCommand) (notifications.AndroidPushReceipt, error) {
	return h.pushDevices().ApplyAndroidPush(ctx, cmd)
}
