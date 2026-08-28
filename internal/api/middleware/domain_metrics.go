package middleware

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Domain-specific Prometheus metrics for Prairie.
var (
	// UserDBPoolOpen tracks the number of currently open user database connections.
	UserDBPoolOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamapp_userdb_pool_open",
		Help: "Number of open user database pool connections.",
	})

	// UserDBRestoreDuration observes the duration of user database restore operations.
	UserDBRestoreDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "streamapp_userdb_restore_duration_seconds",
		Help:    "Duration of user database restore operations in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// UserDBEvictions counts the total number of user database pool evictions.
	UserDBEvictions = promauto.NewCounter(prometheus.CounterOpts{
		Name: "streamapp_userdb_pool_evictions_total",
		Help: "Total number of user database pool evictions.",
	})

	// PlaybackActiveSessions tracks the number of active playback sessions by play method.
	PlaybackActiveSessions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "streamapp_playback_active_sessions",
		Help: "Number of active playback sessions.",
	}, []string{"play_method"})

	// ReconciliationLag tracks the current reconciliation lag in seconds.
	ReconciliationLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "streamapp_reconciliation_lag_seconds",
		Help: "Current reconciliation lag in seconds.",
	})

	// ScannerFiles counts the total number of files processed by the scanner.
	ScannerFiles = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamapp_scanner_files_total",
		Help: "Total number of files processed by the scanner.",
	}, []string{"status"})

	// MatcherResolved counts the total number of resolved matcher operations.
	MatcherResolved = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streamapp_matcher_resolved_total",
		Help: "Total number of resolved matcher operations.",
	}, []string{"step", "provider_slug"})

	// LitestreamSyncErrors counts the total number of Litestream sync errors.
	LitestreamSyncErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "streamapp_litestream_sync_errors_total",
		Help: "Total number of Litestream sync errors.",
	})
)
