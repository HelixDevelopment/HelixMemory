// Package metrics provides Prometheus metrics for HelixMemory operations.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all HelixMemory Prometheus metrics.
type Metrics struct {
	SearchLatency    *prometheus.HistogramVec
	SearchTotal      *prometheus.CounterVec
	AddTotal         *prometheus.CounterVec
	AddLatency       *prometheus.HistogramVec
	ProviderHealth   *prometheus.GaugeVec
	FusionEntries    *prometheus.HistogramVec
	FusionDeduped    *prometheus.CounterVec
	ConsolidationRuns *prometheus.CounterVec
	ConsolidationDuration *prometheus.HistogramVec
	CircuitBreakerState   *prometheus.GaugeVec
	ActiveProviders  prometheus.Gauge
}

// NewMetrics creates and registers all HelixMemory metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		SearchLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "search_latency_seconds",
				Help:      "Search operation latency in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
			},
			[]string{"source", "status"},
		),
		SearchTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "search_total",
				Help:      "Total number of search operations",
			},
			[]string{"source", "status"},
		),
		AddTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "add_total",
				Help:      "Total number of add operations",
			},
			[]string{"source", "type", "status"},
		),
		AddLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "add_latency_seconds",
				Help:      "Add operation latency in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
			},
			[]string{"source", "status"},
		),
		ProviderHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "helixmemory",
				Name:      "provider_healthy",
				Help:      "Provider health status (1=healthy, 0=unhealthy)",
			},
			[]string{"source"},
		),
		FusionEntries: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "fusion_entries_count",
				Help:      "Number of entries in fused results",
				Buckets:   prometheus.LinearBuckets(0, 5, 20),
			},
			[]string{},
		),
		FusionDeduped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "fusion_deduplicated_total",
				Help:      "Total number of deduplicated entries",
			},
			[]string{},
		),
		ConsolidationRuns: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "consolidation_runs_total",
				Help:      "Total number of consolidation runs",
			},
			[]string{"status"},
		),
		ConsolidationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "consolidation_duration_seconds",
				Help:      "Consolidation run duration in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
			},
			[]string{},
		),
		CircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "helixmemory",
				Name:      "circuit_breaker_state",
				Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{"source"},
		),
		ActiveProviders: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "helixmemory",
				Name:      "active_providers",
				Help:      "Number of active memory providers",
			},
		),
	}

	if reg != nil {
		reg.MustRegister(
			m.SearchLatency,
			m.SearchTotal,
			m.AddTotal,
			m.AddLatency,
			m.ProviderHealth,
			m.FusionEntries,
			m.FusionDeduped,
			m.ConsolidationRuns,
			m.ConsolidationDuration,
			m.CircuitBreakerState,
			m.ActiveProviders,
		)
	}

	return m
}

// NoopMetrics returns metrics that are created but not registered anywhere.
// Useful for testing.
func NoopMetrics() *Metrics {
	return NewMetrics(nil)
}
