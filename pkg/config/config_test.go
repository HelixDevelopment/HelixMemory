package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "http://localhost:8283", cfg.LettaEndpoint)
	assert.Equal(t, "http://localhost:8001", cfg.Mem0Endpoint)
	assert.Equal(t, "http://localhost:8000", cfg.CogneeEndpoint)
	assert.Equal(t, "http://localhost:8003", cfg.GraphitiEndpoint)

	assert.Equal(t, "http://localhost:6333", cfg.QdrantEndpoint)
	assert.Equal(t, "bolt://localhost:7687", cfg.Neo4jEndpoint)
	assert.Equal(t, "neo4j", cfg.Neo4jUser)
	assert.Equal(t, "localhost:6379", cfg.RedisEndpoint)

	assert.InDelta(t, 0.92, cfg.FusionDedupThreshold, 0.001)
	assert.InDelta(t, 0.40, cfg.FusionRelevanceWeight, 0.001)
	assert.InDelta(t, 0.25, cfg.FusionRecencyWeight, 0.001)
	assert.InDelta(t, 0.20, cfg.FusionSourceWeight, 0.001)
	assert.InDelta(t, 0.15, cfg.FusionTypeWeight, 0.001)

	assert.True(t, cfg.ConsolidationEnabled)
	assert.Equal(t, 30*time.Minute, cfg.ConsolidationInterval)
	assert.Equal(t, 100, cfg.ConsolidationBatchSize)

	assert.Equal(t, 5, cfg.CircuitBreakerThreshold)
	assert.Equal(t, 30*time.Second, cfg.CircuitBreakerTimeout)

	assert.Equal(t, 10, cfg.DefaultTopK)
	assert.Equal(t, 4, cfg.MaxConcurrentQueries)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
	assert.True(t, cfg.EnableMetrics)

	assert.Equal(t, "text-embedding-3-small", cfg.EmbeddingModel)
	assert.Equal(t, 1536, cfg.EmbeddingDimension)
}

func TestFromEnv(t *testing.T) {
	// Set env vars
	envVars := map[string]string{
		"HELIX_MEMORY_LETTA_ENDPOINT":     "http://letta:9000",
		"HELIX_MEMORY_MEM0_ENDPOINT":      "http://mem0:9001",
		"HELIX_MEMORY_COGNEE_ENDPOINT":    "http://cognee:9002",
		"HELIX_MEMORY_GRAPHITI_ENDPOINT":  "http://graphiti:9003",
		"HELIX_MEMORY_QDRANT_ENDPOINT":    "http://qdrant:9004",
		"HELIX_MEMORY_NEO4J_ENDPOINT":     "bolt://neo4j:9005",
		"HELIX_MEMORY_NEO4J_USER":         "admin",
		"HELIX_MEMORY_NEO4J_PASSWORD":     "secret",
		"HELIX_MEMORY_REDIS_ENDPOINT":     "redis:9006",
		"HELIX_MEMORY_REDIS_PASSWORD":     "redis-secret",
		"HELIX_MEMORY_FUSION_DEDUP_THRESHOLD":      "0.85",
		"HELIX_MEMORY_CONSOLIDATION_ENABLED":        "false",
		"HELIX_MEMORY_CONSOLIDATION_INTERVAL":       "1h",
		"HELIX_MEMORY_DEFAULT_TOP_K":                "20",
		"HELIX_MEMORY_REQUEST_TIMEOUT":              "30s",
		"HELIX_MEMORY_ENABLE_METRICS":               "false",
		"HELIX_MEMORY_EMBEDDING_MODEL":              "nomic-embed",
		"HELIX_MEMORY_EMBEDDING_ENDPOINT":           "http://embed:9007",
		"HELIX_MEMORY_EMBEDDING_DIMENSION":          "768",
		"HELIX_MEMORY_CIRCUIT_BREAKER_THRESHOLD":    "10",
		"HELIX_MEMORY_CIRCUIT_BREAKER_TIMEOUT":      "1m",
	}

	for k, v := range envVars {
		os.Setenv(k, v)
	}
	defer func() {
		for k := range envVars {
			os.Unsetenv(k)
		}
	}()

	cfg := FromEnv()

	assert.Equal(t, "http://letta:9000", cfg.LettaEndpoint)
	assert.Equal(t, "http://mem0:9001", cfg.Mem0Endpoint)
	assert.Equal(t, "http://cognee:9002", cfg.CogneeEndpoint)
	assert.Equal(t, "http://graphiti:9003", cfg.GraphitiEndpoint)
	assert.Equal(t, "http://qdrant:9004", cfg.QdrantEndpoint)
	assert.Equal(t, "bolt://neo4j:9005", cfg.Neo4jEndpoint)
	assert.Equal(t, "admin", cfg.Neo4jUser)
	assert.Equal(t, "secret", cfg.Neo4jPassword)
	assert.Equal(t, "redis:9006", cfg.RedisEndpoint)
	assert.Equal(t, "redis-secret", cfg.RedisPassword)
	assert.InDelta(t, 0.85, cfg.FusionDedupThreshold, 0.001)
	assert.False(t, cfg.ConsolidationEnabled)
	assert.Equal(t, 1*time.Hour, cfg.ConsolidationInterval)
	assert.Equal(t, 20, cfg.DefaultTopK)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.False(t, cfg.EnableMetrics)
	assert.Equal(t, "nomic-embed", cfg.EmbeddingModel)
	assert.Equal(t, "http://embed:9007", cfg.EmbeddingEndpoint)
	assert.Equal(t, 768, cfg.EmbeddingDimension)
	assert.Equal(t, 10, cfg.CircuitBreakerThreshold)
	assert.Equal(t, 1*time.Minute, cfg.CircuitBreakerTimeout)
}

func TestFromEnv_Defaults(t *testing.T) {
	// Ensure no env vars are set
	os.Unsetenv("HELIX_MEMORY_LETTA_ENDPOINT")
	os.Unsetenv("HELIX_MEMORY_MEM0_ENDPOINT")

	cfg := FromEnv()

	// Should use defaults
	assert.Equal(t, "http://localhost:8283", cfg.LettaEndpoint)
	assert.Equal(t, "http://localhost:8001", cfg.Mem0Endpoint)
}

func TestFromEnv_InvalidValues(t *testing.T) {
	os.Setenv("HELIX_MEMORY_FUSION_DEDUP_THRESHOLD", "not-a-number")
	os.Setenv("HELIX_MEMORY_DEFAULT_TOP_K", "not-an-int")
	os.Setenv("HELIX_MEMORY_REQUEST_TIMEOUT", "not-a-duration")
	defer func() {
		os.Unsetenv("HELIX_MEMORY_FUSION_DEDUP_THRESHOLD")
		os.Unsetenv("HELIX_MEMORY_DEFAULT_TOP_K")
		os.Unsetenv("HELIX_MEMORY_REQUEST_TIMEOUT")
	}()

	cfg := FromEnv()

	// Should fall back to defaults
	assert.InDelta(t, 0.92, cfg.FusionDedupThreshold, 0.001)
	assert.Equal(t, 10, cfg.DefaultTopK)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
}
