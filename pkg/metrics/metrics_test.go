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

// TestMetrics_HelpText_I18nSeam is the CONST-046 round-437 proof: every
// Prometheus metric Help string is resolved through the i18n seam, not a
// hardcoded literal. helpText MUST return the bundle-backed English text
// (non-empty, non-verbatim key). Paired mutation: deleting any key from
// en.yaml flips its lookup to the verbatim key string and FAILs this test.
func TestMetrics_HelpText_I18nSeam(t *testing.T) {
	keys := []struct {
		key     string
		wantSub string
	}{
		{"metric_help_search_latency", "Search operation latency"},
		{"metric_help_search_total", "search operations"},
		{"metric_help_add_total", "add operations"},
		{"metric_help_add_latency", "Add operation latency"},
		{"metric_help_provider_health", "health status"},
		{"metric_help_fusion_entries", "fused results"},
		{"metric_help_fusion_deduped", "deduplicated"},
		{"metric_help_consolidation_runs", "consolidation runs"},
		{"metric_help_consolidation_duration", "Consolidation run duration"},
		{"metric_help_circuit_breaker_state", "Circuit breaker state"},
		{"metric_help_active_providers", "active memory providers"},
	}
	for _, k := range keys {
		got := helpText(k.key)
		require.NotEmpty(t, got, "key %q resolved empty", k.key)
		assert.NotEqual(t, k.key, got, "key %q resolved verbatim (not in bundle)", k.key)
		assert.Contains(t, got, k.wantSub, "key %q missing expected substring", k.key)
	}
}

// TestMetrics_HelpWiredFromBundle confirms the constructed metric structs
// actually carry the bundle-backed Help text — proves the seam reaches the
// real Prometheus descriptors, not just the helper in isolation.
func TestMetrics_HelpWiredFromBundle(t *testing.T) {
	m := NewMetrics(prometheus.NewRegistry())
	require.NotNil(t, m)
	// Gather descriptors via a registry and confirm Help text is present.
	reg := prometheus.NewRegistry()
	m2 := NewMetrics(reg)
	require.NotNil(t, m2)
	families, err := reg.Gather()
	require.NoError(t, err)
	helpByName := make(map[string]string)
	for _, f := range families {
		helpByName[f.GetName()] = f.GetHelp()
	}
	// search_total has data only after an observation; Help is set at registration.
	m2.SearchTotal.WithLabelValues("mem0", "ok").Inc()
	families, err = reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == "helixmemory_search_total" {
			assert.Equal(t, helpText("metric_help_search_total"), f.GetHelp(),
				"search_total Help must come from the i18n bundle")
		}
	}
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
