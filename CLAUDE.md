# CLAUDE.md - HelixMemory Module

## Overview

`digital.vasic.helixmemory` is a proprietary unified cognitive memory engine that fuses Mem0, Cognee, and Letta into a single orchestrated system with Graphiti temporal awareness. It implements the `digital.vasic.memory` MemoryStore interface, enabling drop-in replacement for the default Memory module.

**Module**: `digital.vasic.helixmemory` (Go 1.24+)

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│              UnifiedMemoryProvider                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐  │
│  │  Mem0     │ │  Cognee  │ │  Letta   │ │ Graphiti  │  │
│  │ (facts)  │ │ (graphs) │ │ (brain)  │ │(temporal) │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └─────┬─────┘  │
│       │             │            │              │        │
│  ┌────▼─────────────▼────────────▼──────────────▼─────┐ │
│  │              Fusion Engine                          │ │
│  │  Stage 1: Collection & Normalization                │ │
│  │  Stage 2: Deduplication (cosine sim ≥ 0.92)        │ │
│  │  Stage 3: Cross-Source Re-Ranking                   │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌───────────────┐ ┌──────────────┐ ┌────────────────┐  │
│  │    Router     │ │ Consolidation│ │    Metrics     │  │
│  └───────────────┘ └──────────────┘ └────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Component Roles

| Component | Role | Analogy |
|-----------|------|---------|
| Letta | Stateful agent runtime, sleep-time compute | Brain |
| Mem0 | Dynamic fact extraction, preferences | Notebook |
| Cognee | Semantic knowledge graphs (ECL pipeline) | Library |
| Graphiti | Temporal knowledge graph, bi-temporal queries | Timeline |
| UnifiedMemoryProvider | Orchestrator, parallel search, fusion | Nervous System |

## Build & Test

```bash
go build ./...
GOMAXPROCS=2 go test ./... -count=1 -race -p 1
go test -bench=. ./...
```

## Package Structure

| Package | Purpose |
|---------|---------|
| `pkg/types` | Core types: MemoryEntry, MemoryType, MemorySource, interfaces |
| `pkg/config` | Configuration from env vars with defaults |
| `pkg/clients/mem0` | Mem0 REST API client with circuit breaker |
| `pkg/clients/cognee` | Cognee ECL pipeline client with circuit breaker |
| `pkg/clients/letta` | Letta agent runtime client with core memory |
| `pkg/clients/graphiti` | Graphiti temporal graph client |
| `pkg/fusion` | 3-stage fusion engine (collect, dedup, rerank) |
| `pkg/routing` | Intelligent memory routing and classification |
| `pkg/provider` | UnifiedMemoryProvider + MemoryStore adapter |
| `pkg/consolidation` | Sleep-time compute engine |
| `pkg/metrics` | Prometheus metrics |
| `pkg/features/*` | 12 power features |

## Key Interfaces

- `types.MemoryProvider` — Backend contract (Add, Search, Get, Update, Delete, Health)
- `types.CoreMemoryProvider` — Letta core memory (GetCoreMemory, UpdateCoreMemory)
- `types.ConsolidationProvider` — Sleep-time compute (TriggerConsolidation)
- `types.TemporalProvider` — Time-aware queries (SearchTemporal, GetTimeline)
- `provider.MemoryStoreAdapter` — Implements `digital.vasic.memory/pkg/store.MemoryStore`

## Memory Types

- `fact` — Extracted facts (Mem0 primary)
- `graph` — Knowledge graph entries (Cognee primary)
- `core` — Persona/context memory (Letta primary)
- `temporal` — Time-aware memories (Graphiti primary)
- `episodic` — Conversation/event memories
- `procedural` — Learned workflows

## Fusion Engine Weights

```
score = relevance * 0.40 + recency * 0.25 + source * 0.20 + type * 0.15
```

Deduplication threshold: cosine similarity ≥ 0.92

## Infrastructure

Docker Compose: `docker/docker-compose.yml`

Services: Letta (:8283), Mem0 (:8001), Cognee (:8000), Qdrant (:6333), Neo4j (:7474/:7687), Redis (:6379), PostgreSQL (:5432)

## Configuration

All env vars prefixed with `HELIX_MEMORY_`:

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_MEMORY_LETTA_ENDPOINT` | `http://localhost:8283` | Letta server |
| `HELIX_MEMORY_MEM0_ENDPOINT` | `http://localhost:8001` | Mem0 API |
| `HELIX_MEMORY_COGNEE_ENDPOINT` | `http://localhost:8000` | Cognee API |
| `HELIX_MEMORY_GRAPHITI_ENDPOINT` | `http://localhost:8003` | Graphiti API |
| `HELIX_MEMORY_QDRANT_ENDPOINT` | `http://localhost:6333` | Qdrant |
| `HELIX_MEMORY_NEO4J_ENDPOINT` | `bolt://localhost:7687` | Neo4j |
| `HELIX_MEMORY_REDIS_ENDPOINT` | `localhost:6379` | Redis |
| `HELIX_MEMORY_FUSION_DEDUP_THRESHOLD` | `0.92` | Dedup threshold |
| `HELIX_MEMORY_CONSOLIDATION_ENABLED` | `true` | Sleep-time compute |
| `HELIX_MEMORY_CONSOLIDATION_INTERVAL` | `30m` | Consolidation interval |

## Code Style

- Standard Go conventions, `gofmt` formatting
- Imports: stdlib, third-party, internal (blank line separated)
- Line length ≤ 100 chars
- Naming: `camelCase` private, `PascalCase` exported
- Errors: wrap with `fmt.Errorf("...: %w", err)`
- Tests: table-driven, `testify`, `Test<Struct>_<Method>_<Scenario>`
- Conventional Commits: `feat(fusion): add cross-source re-ranking`
