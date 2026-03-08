# HelixMemory - Getting Started

**Module:** `digital.vasic.helixmemory`

## Overview

HelixMemory is a unified cognitive memory engine that fuses four memory
backends -- Mem0, Cognee, Letta, and Graphiti -- into a single orchestrated
system. It implements the `digital.vasic.memory` MemoryStore interface for
drop-in integration with HelixAgent.

## Prerequisites

HelixMemory requires the following infrastructure services:

| Service | Default Port | Purpose |
|---------|-------------|---------|
| Letta | 8283 | Stateful agent runtime |
| Mem0 | 8001 | Fact extraction |
| Cognee | 8000 | Knowledge graphs |
| Graphiti | 8003 | Temporal graph |
| Qdrant | 6333 | Vector similarity search |
| Neo4j | 7687 | Graph database |
| Redis | 6379 | Caching |
| PostgreSQL | 5432 | Persistent storage |

Start all services with the included Docker Compose file:

```bash
cd HelixMemory/docker
docker compose up -d
```

## Quick Start

### 1. Create the Unified Provider

```go
package main

import (
    "context"
    "fmt"

    "digital.vasic.helixmemory/pkg/config"
    "digital.vasic.helixmemory/pkg/clients/mem0"
    "digital.vasic.helixmemory/pkg/clients/cognee"
    "digital.vasic.helixmemory/pkg/clients/letta"
    "digital.vasic.helixmemory/pkg/clients/graphiti"
    "digital.vasic.helixmemory/pkg/provider"
)

func main() {
    cfg := config.FromEnv()

    unified := provider.NewUnifiedMemoryProvider(cfg)

    // Register available backends
    unified.RegisterProvider(mem0.NewClient(cfg.Mem0Endpoint))
    unified.RegisterProvider(cognee.NewClient(cfg.CogneeEndpoint))
    unified.RegisterProvider(letta.NewClient(cfg.LettaEndpoint))
    unified.RegisterProvider(graphiti.NewClient(cfg.GraphitiEndpoint))
```

### 2. Store a Memory

```go
    entry := &types.MemoryEntry{
        Content:    "The user prefers Go for backend development",
        Type:       types.MemoryTypeFact,
        Source:     types.MemorySourceMem0,
        UserID:     "user-123",
        Confidence: 0.92,
        Metadata:   map[string]interface{}{"topic": "preferences"},
    }

    id, err := unified.Add(context.Background(), entry)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Stored memory: %s\n", id)
```

### 3. Search Memories

Search fans out to all registered backends in parallel and fuses results:

```go
    results, err := unified.Search(context.Background(), types.SearchQuery{
        Query:  "programming language preferences",
        UserID: "user-123",
        Limit:  10,
    })
    if err != nil {
        panic(err)
    }

    for _, entry := range results.Entries {
        fmt.Printf("[%.2f] %s (%s via %s)\n",
            entry.Relevance, entry.Content, entry.Type, entry.Source)
    }
}
```

## Memory Types and Routing

The Router automatically classifies memories and routes writes to the
optimal backend:

| Memory Type | Primary Backend | When to Use |
|-------------|----------------|-------------|
| `fact` | Mem0 | User preferences, extracted facts |
| `graph` | Cognee | Knowledge relationships, entity links |
| `core` | Letta | Persona, system context, agent state |
| `temporal` | Graphiti | Time-aware events, history tracking |
| `episodic` | Letta | Conversation history, session logs |
| `procedural` | Cognee | Workflows, step-by-step procedures |

## Using the MemoryStore Adapter

To use HelixMemory as a drop-in replacement for the Memory module:

```go
    adapter := provider.NewMemoryStoreAdapter(unified)
    // adapter implements digital.vasic.memory/pkg/store.MemoryStore
```

## Consolidation (Sleep-Time Compute)

The consolidation engine runs in the background, deduplicating and
enriching memories:

```go
    consolidation.Start(cfg)  // Runs every 30 minutes by default
    status := consolidation.GetStatus()
    fmt.Printf("Runs: %d, Processed: %d\n", status.Runs, status.Processed)
```

## Health Checking

```go
    health := unified.Health(context.Background())
    for provider, status := range health.Providers {
        fmt.Printf("%s: %s\n", provider, status)
    }
```

## Next Steps

- See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design
- See [API_REFERENCE.md](API_REFERENCE.md) for complete type definitions
- See [CONFIGURATION.md](CONFIGURATION.md) for all environment variables
- See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues
