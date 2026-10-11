package jellycompat

import (
	"github.com/prairie-server/prairie-server/internal/config"
	"github.com/prairie-server/prairie-server/internal/noderouting"
	"github.com/prairie-server/prairie-server/internal/playback"
)

func requireCompatWorkerRouting(handler *PlaybackHandler) {
	previous := handler.PlaybackConfig
	handler.PlaybackConfig = func() config.PlaybackConfig {
		var cfg config.PlaybackConfig
		if previous != nil {
			cfg = previous()
		}
		cfg.Routing = config.DefaultPlaybackRoutingPolicy()
		cfg.Routing.RemuxExecution = config.PlaybackExecutionWorkerOnly
		cfg.Routing.VideoTranscodeExecution = config.PlaybackExecutionWorkerOnly
		return cfg
	}
}

func compatLocalVideoAPIRoutingAssignment() *playback.NodeRoutingAssignment {
	return &playback.NodeRoutingAssignment{
		Workload:  string(noderouting.WorkloadVideoTranscode),
		Execution: string(noderouting.ExecutionAPI),
		Egress:    string(noderouting.EgressAPI),
	}
}
