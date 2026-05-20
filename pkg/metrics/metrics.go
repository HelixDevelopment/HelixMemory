// Package metrics provides Prometheus metrics for HelixMemory operations.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"digital.vasic.helixmemory/pkg/i18n"
)

// helpText renders a Prometheus metric Help string for key through the i18n
// seam (CONST-046 round-437). Prometheus `Help:` text is rendered to humans
// in Prometheus/Grafana metric-explorer UIs, so it is a user-facing surface
// and MUST NOT be a hardcoded English literal. The empty locale means
// "translator default"; the bundle — not this call site — owns the text.
func helpText(key string) string {
	return i18n.T("", i18n.BundlePrefix+key)
}

// Metrics holds all HelixMemory Prometheus metrics.
type Metrics struct {
	SearchLatency         *prometheus.HistogramVec
	SearchTotal           *prometheus.CounterVec
	AddTotal              *prometheus.CounterVec
	AddLatency            *prometheus.HistogramVec
	ProviderHealth        *prometheus.GaugeVec
	FusionEntries         *prometheus.HistogramVec
	FusionDeduped         *prometheus.CounterVec
	ConsolidationRuns     *prometheus.CounterVec
	ConsolidationDuration *prometheus.HistogramVec
	CircuitBreakerState   *prometheus.GaugeVec
	ActiveProviders       prometheus.Gauge
}

// NewMetrics creates and registers all HelixMemory metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		SearchLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "search_latency_seconds",
				Help:      helpText("metric_help_search_latency"),
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
			},
			[]string{"source", "status"},
		),
		SearchTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "search_total",
				Help:      helpText("metric_help_search_total"),
			},
			[]string{"source", "status"},
		),
		AddTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "add_total",
				Help:      helpText("metric_help_add_total"),
			},
			[]string{"source", "type", "status"},
		),
		AddLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "add_latency_seconds",
				Help:      helpText("metric_help_add_latency"),
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
			},
			[]string{"source", "status"},
		),
		ProviderHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "helixmemory",
				Name:      "provider_healthy",
				Help:      helpText("metric_help_provider_health"),
			},
			[]string{"source"},
		),
		FusionEntries: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "fusion_entries_count",
				Help:      helpText("metric_help_fusion_entries"),
				Buckets:   prometheus.LinearBuckets(0, 5, 20),
			},
			[]string{},
		),
		FusionDeduped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "fusion_deduplicated_total",
				Help:      helpText("metric_help_fusion_deduped"),
			},
			[]string{},
		),
		ConsolidationRuns: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "helixmemory",
				Name:      "consolidation_runs_total",
				Help:      helpText("metric_help_consolidation_runs"),
			},
			[]string{"status"},
		),
		ConsolidationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "helixmemory",
				Name:      "consolidation_duration_seconds",
				Help:      helpText("metric_help_consolidation_duration"),
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
			},
			[]string{},
		),
		CircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "helixmemory",
				Name:      "circuit_breaker_state",
				Help:      helpText("metric_help_circuit_breaker_state"),
			},
			[]string{"source"},
		),
		ActiveProviders: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "helixmemory",
				Name:      "active_providers",
				Help:      helpText("metric_help_active_providers"),
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
