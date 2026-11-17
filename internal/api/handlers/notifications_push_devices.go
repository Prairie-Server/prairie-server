package handlers

import (
	"github.com/prairie-server/prairie-server/internal/notifications"
)

func (h *NotificationsHandler) pushDevices() *notifications.PushDeviceService {
	if h == nil || h.system == nil {
		return nil
	}
	return h.system.PushDevices
}
