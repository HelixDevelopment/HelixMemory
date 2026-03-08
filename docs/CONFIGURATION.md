# HelixMemory - Configuration

**Module:** `digital.vasic.helixmemory`

## Overview

All configuration is loaded from environment variables prefixed with
`HELIX_MEMORY_`. Missing variables fall back to sensible defaults.
Configuration is loaded via `config.FromEnv()` at startup.

## Backend Endpoints

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_LETTA_ENDPOINT` | `http://localhost:8283` | Letta server URL |
| `HELIX_MEMORY_MEM0_ENDPOINT` | `http://localhost:8001` | Mem0 API URL |
| `HELIX_MEMORY_COGNEE_ENDPOINT` | `http://localhost:8000` | Cognee ECL pipeline URL |
| `HELIX_MEMORY_GRAPHITI_ENDPOINT` | `http://localhost:8003` | Graphiti temporal graph URL |
| `HELIX_MEMORY_QDRANT_ENDPOINT` | `http://localhost:6333` | Qdrant vector DB URL |
| `HELIX_MEMORY_NEO4J_ENDPOINT` | `bolt://localhost:7687` | Neo4j graph DB URL |
| `HELIX_MEMORY_REDIS_ENDPOINT` | `localhost:6379` | Redis cache URL |
| `HELIX_MEMORY_POSTGRES_ENDPOINT` | `localhost:5432` | PostgreSQL URL |

## Fusion Engine

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_FUSION_DEDUP_THRESHOLD` | `0.92` | Deduplication cosine similarity threshold (0.0-1.0) |
| `HELIX_MEMORY_FUSION_RELEVANCE_WEIGHT` | `0.40` | Relevance weight in re-ranking formula |
| `HELIX_MEMORY_FUSION_RECENCY_WEIGHT` | `0.25` | Recency weight in re-ranking formula |
| `HELIX_MEMORY_FUSION_SOURCE_WEIGHT` | `0.20` | Source trust weight in re-ranking formula |
| `HELIX_MEMORY_FUSION_TYPE_WEIGHT` | `0.15` | Memory type weight in re-ranking formula |

**Re-ranking formula:**

```
score = relevance * 0.40 + recency * 0.25 + source * 0.20 + type * 0.15
```

Recency uses exponential decay with approximately 7-day half-life.

## Consolidation (Sleep-Time Compute)

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_CONSOLIDATION_ENABLED` | `true` | Enable background consolidation engine |
| `HELIX_MEMORY_CONSOLIDATION_INTERVAL` | `30m` | Interval between consolidation runs |
| `HELIX_MEMORY_CONSOLIDATION_BATCH_SIZE` | `100` | Memories processed per consolidation run |

## Circuit Breaker

Each backend client is protected by a circuit breaker:

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_CB_FAILURE_THRESHOLD` | `5` | Consecutive failures before circuit opens |
| `HELIX_MEMORY_CB_SUCCESS_THRESHOLD` | `2` | Successes needed to close circuit |
| `HELIX_MEMORY_CB_TIMEOUT` | `30s` | Time in open state before half-open probe |

## Concurrency

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_MAX_CONCURRENT_SEARCHES` | `4` | Max parallel backend searches |
| `HELIX_MEMORY_SEARCH_TIMEOUT` | `10s` | Per-backend search timeout |
| `HELIX_MEMORY_ADD_TIMEOUT` | `5s` | Per-backend add timeout |

## Embedding Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_EMBEDDING_PROVIDER` | `openai` | Embedding provider for deduplication |
| `HELIX_MEMORY_EMBEDDING_MODEL` | `text-embedding-3-small` | Embedding model name |
| `HELIX_MEMORY_EMBEDDING_DIMENSIONS` | `1536` | Embedding vector dimensions |

## Source Trust Scores

These scores influence the re-ranking formula. They are compiled into
the fusion engine and cannot be changed via environment variables:

| Source | Trust Score |
|--------|-------------|
| Letta | 0.95 |
| Graphiti | 0.90 |
| Mem0 | 0.85 |
| Cognee | 0.80 |
| Unknown | 0.50 |

## Infrastructure Services

Default ports for Docker Compose deployment:

| Service | Container Port | Host Port | Protocol |
|---------|---------------|-----------|----------|
| Letta | 8283 | 8283 | HTTP |
| Mem0 | 8001 | 8001 | HTTP |
| Cognee | 8000 | 8000 | HTTP |
| Graphiti | 8003 | 8003 | HTTP |
| Qdrant | 6333 | 6333 | HTTP |
| Neo4j | 7474 / 7687 | 7474 / 7687 | HTTP / Bolt |
| Redis | 6379 | 6379 | TCP |
| PostgreSQL | 5432 | 5432 | TCP |

## Example .env File

```bash
# Backend endpoints
HELIX_MEMORY_LETTA_ENDPOINT=http://localhost:8283
HELIX_MEMORY_MEM0_ENDPOINT=http://localhost:8001
HELIX_MEMORY_COGNEE_ENDPOINT=http://localhost:8000
HELIX_MEMORY_GRAPHITI_ENDPOINT=http://localhost:8003
HELIX_MEMORY_QDRANT_ENDPOINT=http://localhost:6333
HELIX_MEMORY_NEO4J_ENDPOINT=bolt://localhost:7687
HELIX_MEMORY_REDIS_ENDPOINT=localhost:6379

# Fusion engine
HELIX_MEMORY_FUSION_DEDUP_THRESHOLD=0.92

# Consolidation
HELIX_MEMORY_CONSOLIDATION_ENABLED=true
HELIX_MEMORY_CONSOLIDATION_INTERVAL=30m

# Circuit breaker
HELIX_MEMORY_CB_FAILURE_THRESHOLD=5
HELIX_MEMORY_CB_TIMEOUT=30s

# Concurrency
HELIX_MEMORY_MAX_CONCURRENT_SEARCHES=4
HELIX_MEMORY_SEARCH_TIMEOUT=10s
```
