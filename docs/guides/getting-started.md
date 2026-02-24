# Getting Started with HelixMemory

This guide walks you through setting up, configuring, and using the HelixMemory unified cognitive memory engine.

## Prerequisites

- **Go 1.24+** with module support
- **Docker** or **Podman** for infrastructure containers
- **Docker Compose** (v2+) for orchestrating backend services
- Network access to pull container images from Docker Hub

## 1. Start Infrastructure

HelixMemory requires seven infrastructure services. Start them all with Docker Compose from the HelixMemory module root:

```bash
cd HelixMemory/
docker compose -f docker/docker-compose.yml up -d
```

This launches:

| Service    | Port  | Purpose                              |
|------------|-------|--------------------------------------|
| Letta      | 8283  | Stateful agent runtime (the "brain") |
| Mem0       | 8001  | Dynamic fact extraction              |
| Cognee     | 8000  | Semantic knowledge graphs            |
| Qdrant     | 6333  | Vector database for embeddings       |
| Neo4j      | 7474/7687 | Graph database for Cognee/Graphiti |
| Redis      | 6379  | Caching layer                        |
| PostgreSQL | 5432  | Persistent storage for Letta         |

Wait for all services to become healthy:

```bash
docker compose -f docker/docker-compose.yml ps
```

All containers should show `healthy` status. Letta and Cognee may take 30-60 seconds to initialize.

## 2. Configuration

HelixMemory reads all configuration from environment variables prefixed with `HELIX_MEMORY_`. Set these in your shell or `.env` file:

### Backend Endpoints

```bash
export HELIX_MEMORY_LETTA_ENDPOINT=http://localhost:8283
export HELIX_MEMORY_MEM0_ENDPOINT=http://localhost:8001
export HELIX_MEMORY_COGNEE_ENDPOINT=http://localhost:8000
export HELIX_MEMORY_GRAPHITI_ENDPOINT=http://localhost:8003
```

### Infrastructure

```bash
export HELIX_MEMORY_QDRANT_ENDPOINT=http://localhost:6333
export HELIX_MEMORY_NEO4J_ENDPOINT=bolt://localhost:7687
export HELIX_MEMORY_NEO4J_USER=neo4j
export HELIX_MEMORY_NEO4J_PASSWORD=helixmemory
export HELIX_MEMORY_REDIS_ENDPOINT=localhost:6379
```

### Fusion Engine

```bash
export HELIX_MEMORY_FUSION_DEDUP_THRESHOLD=0.92   # Cosine similarity threshold
```

### Consolidation (Sleep-Time Compute)

```bash
export HELIX_MEMORY_CONSOLIDATION_ENABLED=true
export HELIX_MEMORY_CONSOLIDATION_INTERVAL=30m
```

### General

```bash
export HELIX_MEMORY_DEFAULT_TOP_K=10
export HELIX_MEMORY_REQUEST_TIMEOUT=10s
export HELIX_MEMORY_ENABLE_METRICS=true
```

All variables have sensible defaults. If you started infrastructure with the default Docker Compose file, no configuration changes are needed.

## 3. Building

HelixMemory is a Go module imported by HelixAgent. To build it standalone:

```bash
cd HelixMemory/
go build ./...
```

To run tests:

```bash
GOMAXPROCS=2 go test ./... -count=1 -race -p 1
```

To run benchmarks:

```bash
go test -bench=. ./...
```

When used as part of HelixAgent, the module is imported via a `replace` directive in the root `go.mod`:

```
replace digital.vasic.helixmemory => ./HelixMemory
```

## 4. Basic Usage

### Initialize the Provider

```go
package main

import (
    "context"
    "fmt"
    "log"

    "digital.vasic.helixmemory/pkg/config"
    "digital.vasic.helixmemory/pkg/provider"
    "digital.vasic.helixmemory/pkg/clients/mem0"
    "digital.vasic.helixmemory/pkg/clients/letta"
    "digital.vasic.helixmemory/pkg/clients/cognee"
    "digital.vasic.helixmemory/pkg/types"
)

func main() {
    ctx := context.Background()

    // Load configuration from environment
    cfg := config.FromEnv()

    // Create the unified provider
    unified := provider.New(cfg)

    // Register backend clients
    unified.RegisterProvider(mem0.NewClient(cfg))
    unified.RegisterProvider(letta.NewClient(cfg))
    unified.RegisterProvider(cognee.NewClient(cfg))
    // graphiti.NewClient(cfg) can be added when Graphiti is available

    // Verify health
    if err := unified.Health(ctx); err != nil {
        log.Fatalf("Health check failed: %v", err)
    }
    fmt.Println("HelixMemory is healthy")
}
```

### Add a Memory

```go
entry := &types.MemoryEntry{
    Content: "The user prefers Go interfaces for abstraction and uses testify for testing.",
    Type:    types.MemoryTypeFact,
    UserID:  "user-123",
    Tags:    []string{"preferences", "golang"},
}

if err := unified.Add(ctx, entry); err != nil {
    log.Fatalf("Failed to add memory: %v", err)
}
fmt.Println("Memory stored successfully")
```

The router automatically selects the best backend. For `fact` type memories, it routes to Mem0. For `core` type, it routes to Letta. For `graph` type, it routes to Cognee.

### Search Memories

```go
req := &types.SearchRequest{
    Query:  "user coding preferences",
    UserID: "user-123",
    TopK:   5,
}

result, err := unified.Search(ctx, req)
if err != nil {
    log.Fatalf("Search failed: %v", err)
}

fmt.Printf("Found %d results from %v in %v\n",
    result.Total, result.Sources, result.Duration)

for _, entry := range result.Entries {
    fmt.Printf("  [%s/%s] (%.2f) %s\n",
        entry.Source, entry.Type, entry.Relevance, entry.Content)
}
```

Search queries all registered backends in parallel, then fuses results through the 3-stage pipeline (collect, deduplicate, re-rank).

### Get a Memory by ID

```go
entry, err := unified.Get(ctx, "some-memory-id")
if err != nil {
    log.Fatalf("Get failed: %v", err)
}
fmt.Printf("Memory: %s (source: %s, confidence: %.2f)\n",
    entry.Content, entry.Source, entry.Confidence)
```

Get tries all registered providers until one returns the entry.

### Get Memory History

```go
entries, err := unified.GetHistory(ctx, "user-123", 20)
if err != nil {
    log.Fatalf("History failed: %v", err)
}

for _, entry := range entries {
    fmt.Printf("  [%s] %s — %s\n",
        entry.CreatedAt.Format("2006-01-02"), entry.Source, entry.Content)
}
```

History is fetched from all backends in parallel, merged, and sorted by creation time (newest first).

## 5. Health Check Verification

### Aggregate Health

```go
if err := unified.Health(ctx); err != nil {
    fmt.Printf("UNHEALTHY: %v\n", err)
} else {
    fmt.Println("HEALTHY: at least one provider is operational")
}
```

The aggregate health check passes if at least one registered provider is healthy.

### Detailed Health

```go
healthMap := unified.HealthDetailed(ctx)
for source, err := range healthMap {
    status := "OK"
    if err != nil {
        status = err.Error()
    }
    fmt.Printf("  %s: %s\n", source, status)
}
```

This checks all providers in parallel and returns individual health status for each.

### Direct Health Endpoints

Each backend exposes a health endpoint that the client checks:

| Backend  | Health Endpoint                    |
|----------|------------------------------------|
| Letta    | `GET http://localhost:8283/v1/health/`      |
| Mem0     | `GET http://localhost:8001/health`           |
| Cognee   | `GET http://localhost:8000/api/v1/health`    |
| Qdrant   | `GET http://localhost:6333/healthz`          |

You can verify these directly:

```bash
curl -s http://localhost:8283/v1/health/
curl -s http://localhost:8001/health
curl -s http://localhost:8000/api/v1/health
curl -s http://localhost:6333/healthz
```

## 6. Using the MemoryStore Adapter

To use HelixMemory as a drop-in replacement for the `digital.vasic.memory` module's `MemoryStore` interface:

```go
import (
    "digital.vasic.helixmemory/pkg/provider"
    modstore "digital.vasic.memory/pkg/store"
)

// Create the unified provider (as shown above)
unified := provider.New(cfg)
// ... register providers ...

// Wrap with the adapter
adapter := provider.NewMemoryStoreAdapter(unified)

// Now use it anywhere a MemoryStore is expected
var store modstore.MemoryStore = adapter

// Standard MemoryStore operations work transparently
store.Add(ctx, &modstore.Memory{
    Content: "Some fact to remember",
    Scope:   modstore.ScopeUser,
})

results, _ := store.Search(ctx, "remembered fact", nil)
```

The adapter translates between `digital.vasic.memory` types and HelixMemory types, preserving metadata and scope information in both directions.

## 7. Stopping Infrastructure

```bash
docker compose -f docker/docker-compose.yml down
```

To also remove persisted data volumes:

```bash
docker compose -f docker/docker-compose.yml down -v
```

## Next Steps

- Read the [Architecture Guide](architecture.md) for a deep dive into the fusion pipeline and routing logic.
- Read the [Power Features Guide](power-features.md) to learn about all 12 advanced features.
- Explore the SQL schema in `docs/sql/schema.sql` for the relational data model.
