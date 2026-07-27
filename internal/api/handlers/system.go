package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prairie-server/prairie-server/internal/buildinfo"
	"github.com/prairie-server/prairie-server/internal/nodepool"
	"github.com/prairie-server/prairie-server/internal/playback"
)

// SystemHandler serves read-only system inspection endpoints.
type SystemHandler struct {
	transcodePool *nodepool.TranscodePool
	jwtSecret     string
	ffmpegPath    string
	buildInfo     buildinfo.Info
	updateChecker *buildinfo.UpdateChecker
}

// NewSystemHandler creates a SystemHandler.
func NewSystemHandler(transcodePool *nodepool.TranscodePool, jwtSecret string, ffmpegPath string) *SystemHandler {
	return &SystemHandler{
		transcodePool: transcodePool,
		jwtSecret:     jwtSecret,
		ffmpegPath:    ffmpegPath,
		buildInfo:     buildinfo.Current(),
		updateChecker: buildinfo.DefaultUpdateChecker,
	}
}

// HandleHWAccel handles GET /admin/system/hw-accel.
// When transcode nodes are registered it delegates to the first healthy node.
// Otherwise it probes the local host.
func (h *SystemHandler) HandleHWAccel(w http.ResponseWriter, r *http.Request) {
	if h.transcodePool != nil {
		if node := h.transcodePool.Acquire(); node != nil {
			info, err := h.fetchRemoteHWAccel(r.Context(), node)
			if err == nil {
				writeJSON(w, http.StatusOK, info)
				return
			}
			slog.WarnContext(r.Context(), "hw-accel: remote node probe failed, falling back to local", "component", "api",
				"node", node.URL, "error", err)
		}
	}

	info := playback.DetectHWAccelWithFFmpeg(h.ffmpegPath)
	writeJSON(w, http.StatusOK, info)
}

// HandleBuildInfo handles GET /admin/system/build.
func (h *SystemHandler) HandleBuildInfo(w http.ResponseWriter, r *http.Request) {
	info := h.buildInfo
	if h.updateChecker != nil {
		info = h.updateChecker.Enrich(r.Context(), info)
	} else {
		info.UpdateStatus = buildinfo.UpdateStatusUnknown
		info.ChangelogURL = buildinfo.DefaultChangelogURL
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *SystemHandler) fetchRemoteHWAccel(ctx context.Context, node *nodepool.Node) (playback.HWAccelInfo, error) {
	return fetchRemoteTranscodeCapabilities(ctx, node.URL, h.jwtSecret)
}
