# HelixMemory

**Unified Cognitive Memory Engine** — Fusing Mem0, Cognee, and Letta into a single orchestrated memory system for AI applications.

## What is HelixMemory?

HelixMemory is a proprietary Go module that combines three best-in-class memory systems into a unified cognitive memory engine:

- **Mem0** — Dynamic fact extraction and preference management (26%+ accuracy over baseline)
- **Cognee** — Semantic knowledge graphs via ECL pipelines (38+ data source connectors)
- **Letta** — Stateful agent runtime with sleep-time compute and editable memory blocks
- **Graphiti** — Temporal knowledge graph with bi-temporal data modeling

Instead of replacing individual systems, HelixMemory **orchestrates** them — each backend handles what it does best, and a fusion engine combines their results into a unified memory experience.

## Key Features

- **Parallel search** across all backends with automatic fusion
- **3-stage Fusion Engine**: Collection → Deduplication → Cross-Source Re-Ranking
- **Intelligent routing** — memories classified and routed to the optimal backend
- **Graceful degradation** — if a backend is down, the rest continue serving
- **Circuit breakers** for all backend connections
- **Sleep-time compute** — consolidation during idle periods
- **MemoryStore interface** — drop-in replacement for `digital.vasic.memory`
- **Prometheus metrics** for full observability

## Quick Start

```bash
# Start infrastructure
cd docker && docker compose up -d

# Run tests
go test ./... -race
```

## Architecture

```
┌─────────────────────────────────────────┐
│       UnifiedMemoryProvider              │
│                                          │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌────────┐ │
│  │ Mem0 │ │Cognee│ │Letta │ │Graphiti│ │
│  └──┬───┘ └──┬───┘ └──┬───┘ └───┬────┘ │
│     └────────┴────────┴─────────┘       │
│              Fusion Engine               │
│     ┌────────┬──────────┬──────┐        │
│     │Collect │Deduplicate│Rerank│        │
│     └────────┴──────────┴──────┘        │
└─────────────────────────────────────────┘
```

## Integration with HelixAgent

HelixMemory implements `digital.vasic.memory/pkg/store.MemoryStore`, making it a drop-in replacement. When the HelixMemory submodule is present, HelixAgent automatically uses it instead of the default Memory module.

## License

Proprietary — HelixDevelopment Organization
