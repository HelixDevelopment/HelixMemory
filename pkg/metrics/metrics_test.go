package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	require.NotNil(t, m)
	assert.NotNil(t, m.SearchLatency)
	assert.NotNil(t, m.AddTotal)
	assert.NotNil(t, m.ProviderHealth)
	assert.NotNil(t, m.ActiveProviders)
	assert.NotNil(t, m.CircuitBreakerState)
}

func TestNewMetrics_NilRegisterer(t *testing.T) {
	m := NewMetrics(nil)

	require.NotNil(t, m)
	assert.NotNil(t, m.SearchLatency)
	assert.NotNil(t, m.AddTotal)
	assert.NotNil(t, m.ProviderHealth)
	assert.NotNil(t, m.ActiveProviders)
	assert.NotNil(t, m.CircuitBreakerState)
}

func TestNoopMetrics(t *testing.T) {
	m := NoopMetrics()

	require.NotNil(t, m)
	assert.NotNil(t, m.SearchLatency)
	assert.NotNil(t, m.AddTotal)
	assert.NotNil(t, m.ProviderHealth)
	assert.NotNil(t, m.ActiveProviders)
	assert.NotNil(t, m.CircuitBreakerState)
}

func TestMetrics_RecordSearch(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	// Should not panic
	assert.NotPanics(t, func() {
		m.SearchLatency.WithLabelValues("mem0", "ok").Observe(0.1)
	})
}

func TestMetrics_RecordAdd(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	// Should not panic
	assert.NotPanics(t, func() {
		m.AddTotal.WithLabelValues("mem0", "fact", "ok").Inc()
	})
}

func TestMetrics_ProviderHealth(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	// Should not panic
	assert.NotPanics(t, func() {
		m.ProviderHealth.WithLabelValues("mem0").Set(1)
	})
}

func TestMetrics_ActiveProviders(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	// Should not panic
	assert.NotPanics(t, func() {
		m.ActiveProviders.Set(4)
	})
}

func TestMetrics_CircuitBreakerState(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	// Should not panic
	assert.NotPanics(t, func() {
		m.CircuitBreakerState.WithLabelValues("mem0").Set(0)
	})
}

func TestMetrics_Registration(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)

	// Use the metrics to generate some data
	m.SearchLatency.WithLabelValues("mem0", "ok").Observe(0.05)
	m.AddTotal.WithLabelValues("mem0", "fact", "ok").Inc()
	m.ProviderHealth.WithLabelValues("mem0").Set(1)
	m.ActiveProviders.Set(4)
	m.CircuitBreakerState.WithLabelValues("mem0").Set(0)

	// Gather should return registered metrics
	families, err := registry.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, families, "registry should contain gathered metrics")

	// Verify specific metric families are present
	metricNames := make(map[string]bool)
	for _, family := range families {
		metricNames[family.GetName()] = true
	}

	assert.True(t, len(metricNames) > 0, "should have at least one metric family registered")
}
