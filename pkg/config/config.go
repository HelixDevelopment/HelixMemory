// Package config provides configuration management for HelixMemory.
// Supports both local container deployment and cloud API modes.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all HelixMemory configuration.
type Config struct {
	// Mode: "local" (containers) or "cloud" (API)
	Mode string

	// Local Endpoints
	CogneeEndpoint   string
	Mem0Endpoint     string
	LettaEndpoint    string
	GraphitiEndpoint string

	// Cloud API Keys (for fallback)
	CogneeAPIKey   string
	Mem0APIKey     string
	LettaAPIKey    string

	// Cloud Endpoints (optional, uses defaults if empty)
	CogneeCloudEndpoint   string
	Mem0CloudEndpoint     string
	LettaCloudEndpoint    string

	// Organization/Project IDs (for cloud APIs)
	Mem0OrgID     string
	Mem0ProjectID string

	// Infrastructure
	QdrantEndpoint   string
	Neo4jEndpoint    string
	Neo4jUser        string
	Neo4jPassword    string
	RedisEndpoint    string
	RedisPassword    string

	// Fusion Engine
	FusionDedupThreshold float64
	DefaultTopK          int
	RequestTimeout       time.Duration

	// Consolidation
	ConsolidationEnabled  bool
	ConsolidationInterval time.Duration

	// Circuit Breaker
	CircuitBreakerThreshold int
	CircuitBreakerTimeout   time.Duration

	// Embedding
	EmbeddingModel      string
	EmbeddingEndpoint   string
	EmbeddingDimension  int

	// Metrics
	EnableMetrics bool
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		// Default values
		Mode:                    getEnv("HELIX_MEMORY_MODE", "local"),
		FusionDedupThreshold:    getEnvFloat("HELIX_MEMORY_FUSION_DEDUP_THRESHOLD", 0.92),
		DefaultTopK:             getEnvInt("HELIX_MEMORY_DEFAULT_TOP_K", 10),
		RequestTimeout:          getEnvDuration("HELIX_MEMORY_REQUEST_TIMEOUT", 10*time.Second),
		ConsolidationEnabled:    getEnvBool("HELIX_MEMORY_CONSOLIDATION_ENABLED", true),
		ConsolidationInterval:   getEnvDuration("HELIX_MEMORY_CONSOLIDATION_INTERVAL", 30*time.Minute),
		CircuitBreakerThreshold: getEnvInt("HELIX_MEMORY_CIRCUIT_BREAKER_THRESHOLD", 5),
		CircuitBreakerTimeout:   getEnvDuration("HELIX_MEMORY_CIRCUIT_BREAKER_TIMEOUT", 30*time.Second),
		EmbeddingModel:          getEnv("HELIX_MEMORY_EMBEDDING_MODEL", "text-embedding-3-small"),
		EmbeddingEndpoint:       getEnv("HELIX_MEMORY_EMBEDDING_ENDPOINT", "http://localhost:7061/v1"),
		EmbeddingDimension:      getEnvInt("HELIX_MEMORY_EMBEDDING_DIMENSION", 1536),
		EnableMetrics:           getEnvBool("HELIX_MEMORY_ENABLE_METRICS", true),
	}

	// Load local endpoints
	cfg.CogneeEndpoint = getEnv("HELIX_MEMORY_COGNEE_ENDPOINT", "http://localhost:8000")
	cfg.Mem0Endpoint = getEnv("HELIX_MEMORY_MEM0_ENDPOINT", "http://localhost:8001")
	cfg.LettaEndpoint = getEnv("HELIX_MEMORY_LETTA_ENDPOINT", "http://localhost:8283")
	cfg.GraphitiEndpoint = getEnv("HELIX_MEMORY_GRAPHITI_ENDPOINT", "http://localhost:8003")

	// Load cloud API keys (sensitive - never log these)
	cfg.CogneeAPIKey = getEnv("HELIX_MEMORY_COGNEE_API_KEY", "")
	cfg.Mem0APIKey = getEnv("HELIX_MEMORY_MEM0_API_KEY", "")
	cfg.LettaAPIKey = getEnv("HELIX_MEMORY_LETTA_API_KEY", "")

	// Load cloud endpoints (with defaults)
	cfg.CogneeCloudEndpoint = getEnv("HELIX_MEMORY_COGNEE_CLOUD_ENDPOINT", "https://api.cognee.ai")
	cfg.Mem0CloudEndpoint = getEnv("HELIX_MEMORY_MEM0_CLOUD_ENDPOINT", "https://api.mem0.ai")
	cfg.LettaCloudEndpoint = getEnv("HELIX_MEMORY_LETTA_CLOUD_ENDPOINT", "https://api.letta.com")

	// Organization IDs
	cfg.Mem0OrgID = getEnv("HELIX_MEMORY_MEM0_ORG_ID", "")
	cfg.Mem0ProjectID = getEnv("HELIX_MEMORY_MEM0_PROJECT_ID", "")

	// Infrastructure
	cfg.QdrantEndpoint = getEnv("HELIX_MEMORY_QDRANT_ENDPOINT", "http://localhost:6333")
	cfg.Neo4jEndpoint = getEnv("HELIX_MEMORY_NEO4J_ENDPOINT", "bolt://localhost:7687")
	cfg.Neo4jUser = getEnv("HELIX_MEMORY_NEO4J_USER", "neo4j")
	cfg.Neo4jPassword = getEnv("HELIX_MEMORY_NEO4J_PASSWORD", "helixmemory")
	cfg.RedisEndpoint = getEnv("HELIX_MEMORY_REDIS_ENDPOINT", "localhost:6379")
	cfg.RedisPassword = getEnv("HELIX_MEMORY_REDIS_PASSWORD", "")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Mode != "local" && c.Mode != "cloud" {
		return fmt.Errorf("invalid mode: %s (must be 'local' or 'cloud')", c.Mode)
	}

	// In cloud mode, API keys are required
	if c.Mode == "cloud" {
		if c.CogneeAPIKey == "" && c.Mem0APIKey == "" && c.LettaAPIKey == "" {
			return fmt.Errorf("cloud mode requires at least one API key (COGNEE_API_KEY, MEM0_API_KEY, or LETTA_API_KEY)")
		}
	}

	return nil
}

// GetCogneeEndpoint returns the appropriate Cognee endpoint based on mode.
func (c *Config) GetCogneeEndpoint() string {
	if c.Mode == "cloud" && c.CogneeAPIKey != "" {
		return c.CogneeCloudEndpoint
	}
	return c.CogneeEndpoint
}

// GetMem0Endpoint returns the appropriate Mem0 endpoint based on mode.
func (c *Config) GetMem0Endpoint() string {
	if c.Mode == "cloud" && c.Mem0APIKey != "" {
		return c.Mem0CloudEndpoint
	}
	return c.Mem0Endpoint
}

// GetLettaEndpoint returns the appropriate Letta endpoint based on mode.
func (c *Config) GetLettaEndpoint() string {
	if c.Mode == "cloud" && c.LettaAPIKey != "" {
		return c.LettaCloudEndpoint
	}
	return c.LettaEndpoint
}

// IsCloudMode returns true if running in cloud mode.
func (c *Config) IsCloudMode() bool {
	return c.Mode == "cloud"
}

// HasCognee returns true if Cognee is configured.
func (c *Config) HasCognee() bool {
	return c.CogneeAPIKey != "" || c.Mode == "local"
}

// HasMem0 returns true if Mem0 is configured.
func (c *Config) HasMem0() bool {
	return c.Mem0APIKey != "" || c.Mode == "local"
}

// HasLetta returns true if Letta is configured.
func (c *Config) HasLetta() bool {
	return c.LettaAPIKey != "" || c.Mode == "local"
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
