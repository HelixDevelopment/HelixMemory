// Package config provides configuration for the HelixMemory system.
// It reads from environment variables and provides sensible defaults.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the complete HelixMemory configuration.
type Config struct {
	// Backend endpoints
	LettaEndpoint   string `json:"letta_endpoint"`
	Mem0Endpoint    string `json:"mem0_endpoint"`
	CogneeEndpoint  string `json:"cognee_endpoint"`
	GraphitiEndpoint string `json:"graphiti_endpoint"`

	// Infrastructure
	QdrantEndpoint string `json:"qdrant_endpoint"`
	Neo4jEndpoint  string `json:"neo4j_endpoint"`
	Neo4jUser      string `json:"neo4j_user"`
	Neo4jPassword  string `json:"neo4j_password"`
	RedisEndpoint  string `json:"redis_endpoint"`
	RedisPassword  string `json:"redis_password"`

	// Fusion engine
	FusionDedupThreshold   float64 `json:"fusion_dedup_threshold"`
	FusionRelevanceWeight  float64 `json:"fusion_relevance_weight"`
	FusionRecencyWeight    float64 `json:"fusion_recency_weight"`
	FusionSourceWeight     float64 `json:"fusion_source_weight"`
	FusionTypeWeight       float64 `json:"fusion_type_weight"`

	// Consolidation (sleep-time compute)
	ConsolidationEnabled  bool          `json:"consolidation_enabled"`
	ConsolidationInterval time.Duration `json:"consolidation_interval"`
	ConsolidationBatchSize int          `json:"consolidation_batch_size"`

	// Circuit breaker
	CircuitBreakerThreshold int           `json:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   time.Duration `json:"circuit_breaker_timeout"`

	// General
	DefaultTopK        int           `json:"default_top_k"`
	MaxConcurrentQueries int         `json:"max_concurrent_queries"`
	RequestTimeout     time.Duration `json:"request_timeout"`
	EnableMetrics      bool          `json:"enable_metrics"`

	// Embedding
	EmbeddingModel     string `json:"embedding_model"`
	EmbeddingEndpoint  string `json:"embedding_endpoint"`
	EmbeddingDimension int    `json:"embedding_dimension"`
}

// DefaultConfig returns the default configuration with standard values.
func DefaultConfig() *Config {
	return &Config{
		LettaEndpoint:   "http://localhost:8283",
		Mem0Endpoint:    "http://localhost:8001",
		CogneeEndpoint:  "http://localhost:8000",
		GraphitiEndpoint: "http://localhost:8003",

		QdrantEndpoint: "http://localhost:6333",
		Neo4jEndpoint:  "bolt://localhost:7687",
		Neo4jUser:      "neo4j",
		Neo4jPassword:  "password",
		RedisEndpoint:  "localhost:6379",
		RedisPassword:  "",

		FusionDedupThreshold:  0.92,
		FusionRelevanceWeight: 0.40,
		FusionRecencyWeight:   0.25,
		FusionSourceWeight:    0.20,
		FusionTypeWeight:      0.15,

		ConsolidationEnabled:   true,
		ConsolidationInterval:  30 * time.Minute,
		ConsolidationBatchSize: 100,

		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,

		DefaultTopK:          10,
		MaxConcurrentQueries: 4,
		RequestTimeout:       10 * time.Second,
		EnableMetrics:        true,

		EmbeddingModel:     "text-embedding-3-small",
		EmbeddingEndpoint:  "http://localhost:7061/v1",
		EmbeddingDimension: 1536,
	}
}

// FromEnv loads configuration from environment variables, using defaults
// for any unset values.
func FromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("HELIX_MEMORY_LETTA_ENDPOINT"); v != "" {
		cfg.LettaEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_MEM0_ENDPOINT"); v != "" {
		cfg.Mem0Endpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_COGNEE_ENDPOINT"); v != "" {
		cfg.CogneeEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_GRAPHITI_ENDPOINT"); v != "" {
		cfg.GraphitiEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_QDRANT_ENDPOINT"); v != "" {
		cfg.QdrantEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_NEO4J_ENDPOINT"); v != "" {
		cfg.Neo4jEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_NEO4J_USER"); v != "" {
		cfg.Neo4jUser = v
	}
	if v := os.Getenv("HELIX_MEMORY_NEO4J_PASSWORD"); v != "" {
		cfg.Neo4jPassword = v
	}
	if v := os.Getenv("HELIX_MEMORY_REDIS_ENDPOINT"); v != "" {
		cfg.RedisEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_REDIS_PASSWORD"); v != "" {
		cfg.RedisPassword = v
	}
	if v := os.Getenv("HELIX_MEMORY_FUSION_DEDUP_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.FusionDedupThreshold = f
		}
	}
	if v := os.Getenv("HELIX_MEMORY_CONSOLIDATION_ENABLED"); v != "" {
		cfg.ConsolidationEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("HELIX_MEMORY_CONSOLIDATION_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ConsolidationInterval = d
		}
	}
	if v := os.Getenv("HELIX_MEMORY_DEFAULT_TOP_K"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DefaultTopK = n
		}
	}
	if v := os.Getenv("HELIX_MEMORY_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.RequestTimeout = d
		}
	}
	if v := os.Getenv("HELIX_MEMORY_ENABLE_METRICS"); v != "" {
		cfg.EnableMetrics = v == "true" || v == "1"
	}
	if v := os.Getenv("HELIX_MEMORY_EMBEDDING_MODEL"); v != "" {
		cfg.EmbeddingModel = v
	}
	if v := os.Getenv("HELIX_MEMORY_EMBEDDING_ENDPOINT"); v != "" {
		cfg.EmbeddingEndpoint = v
	}
	if v := os.Getenv("HELIX_MEMORY_EMBEDDING_DIMENSION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.EmbeddingDimension = n
		}
	}
	if v := os.Getenv("HELIX_MEMORY_CIRCUIT_BREAKER_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.CircuitBreakerThreshold = n
		}
	}
	if v := os.Getenv("HELIX_MEMORY_CIRCUIT_BREAKER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CircuitBreakerTimeout = d
		}
	}

	return cfg
}
