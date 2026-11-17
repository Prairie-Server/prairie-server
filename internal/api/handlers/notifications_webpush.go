package handlers

import (
	"github.com/prairie-server/prairie-server/internal/notifications"
)

func (h *NotificationsHandler) webPush() *notifications.WebPushService {
	if h == nil || h.system == nil {
		return nil
	}
	return h.system.WebPush
}
