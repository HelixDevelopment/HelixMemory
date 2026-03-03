# HelixMemory Module Architecture

**Module:** `digital.vasic.helixmemory`

## Overview

HelixMemory is a unified cognitive memory engine that fuses four best-in-class memory systems -- Mem0, Cognee, Letta, and Graphiti -- into a single orchestrated system. Instead of replacing individual backends, HelixMemory orchestrates them: each backend handles what it does best, and a 3-stage fusion engine combines their results into a unified memory experience. HelixMemory implements the `digital.vasic.memory` MemoryStore interface, enabling drop-in replacement for the default Memory module in HelixAgent.

## Core Components

### UnifiedMemoryProvider (`pkg/provider/unified.go`)

The nervous system of HelixMemory. Orchestrates all registered memory backends through a single interface. Performs parallel search across all available providers using `errgroup`, fuses results through the fusion engine, and degrades gracefully when backends are unavailable.

**Key responsibilities:**
- Provider registration and lifecycle management
- Parallel search with configurable concurrency limits
- Intelligent write routing via the Router
- Graceful degradation (backend failures logged, not propagated)
- Delegated core memory (Letta), temporal queries (Graphiti), and consolidation

### Fusion Engine (`pkg/fusion/engine.go`)

The 3-stage pipeline that combines search results from multiple backends into a single, high-quality result set.

**Stage 1 -- Collection and Normalization:** Gathers all entries from backend search results. Normalizes relevance scores to the [0, 1] range.

**Stage 2 -- Deduplication:** Removes near-duplicate entries using cosine similarity on embeddings (preferred) or Jaccard similarity on content tokens (fallback). Default threshold: 0.92. When duplicates are found, the entry with higher confidence is kept.

**Stage 3 -- Cross-Source Re-Ranking:** Computes a composite score for each entry and re-sorts:
```
score = relevance * 0.40 + recency * 0.25 + source * 0.20 + type * 0.15
```
Recency uses exponential decay with a ~7-day half-life. Source trust scores range from 0.50 (unknown) to 0.95 (Letta). Type scores reflect the cognitive priority of each memory type relative to the query.

### Router (`pkg/routing/router.go`)

Classifies memory operations and routes them to the appropriate backend(s). Write operations go to a single primary backend based on memory type. Read operations fan out to all available backends for comprehensive search.

**Write routing (type -> primary backend):**

| Memory Type | Primary Backend | Rationale |
|-------------|----------------|-----------|
| `fact` | Mem0 | Dynamic fact extraction |
| `graph` | Cognee | Knowledge graph relations |
| `core` | Letta | Stateful agent memory |
| `temporal` | Graphiti | Time-aware storage |
| `episodic` | Letta | Conversation history |
| `procedural` | Cognee | Workflow patterns |

**Content-based classification:** The Router analyzes content for keyword signals (temporal indicators, procedural indicators, relationship indicators, persona indicators, episodic indicators) to auto-classify untyped memories.

### Backend Clients (`pkg/clients/`)

Four REST API clients, each implementing the `MemoryProvider` interface with circuit breaker protection:

- **Mem0 Client** (`clients/mem0/`) -- Communicates with the Mem0 REST API for fact extraction and preference management. Maps between Mem0's `mem0Memory` format and HelixMemory's `MemoryEntry`.
- **Cognee Client** (`clients/cognee/`) -- Communicates with the Cognee ECL pipeline API for knowledge graph operations. Supports graph-based search (insights, summaries) and data ingestion.
- **Letta Client** (`clients/letta/`) -- Communicates with the Letta server for stateful agent memory. Extends `MemoryProvider` with `CoreMemoryProvider` for editable in-context memory blocks.
- **Graphiti Client** (`clients/graphiti/`) -- Communicates with the Graphiti API for temporal graph operations. Extends `MemoryProvider` with `TemporalProvider` for bi-temporal queries and edge invalidation.

### Consolidation Engine (`pkg/consolidation/consolidation.go`)

Sleep-time compute engine inspired by Letta's innovation of doing useful work while agents are idle. Runs on a configurable interval (default: 30 minutes) and performs three phases:

1. **Collect** -- Gather recent memories from all registered providers
2. **Deduplicate** -- Identify and remove cross-backend duplicates by ID
3. **Enrich** -- Cross-reference and add consolidation metadata

Tracks statistics (runs, processed, deduplicated, errors) and exposes status via `ConsolidationStatus`.

### MemoryStore Adapter (`pkg/provider/adapter.go`)

Bridge that implements `digital.vasic.memory/pkg/store.MemoryStore`, making HelixMemory a drop-in replacement for the default Memory module. Converts between HelixMemory's `MemoryEntry` and the Memory module's `Memory` type, preserving metadata, scope, source, and confidence information bidirectionally.

### Infrastructure Bridge (`pkg/infra/`)

Abstracts container infrastructure operations for HelixMemory's 7 required services. Defines the `InfraProvider` interface that can be implemented by the `digital.vasic.containers` module or by a local docker-compose fallback. Manages service endpoints, health aggregation, and local/remote deployment awareness.

### Configuration (`pkg/config/config.go`)

Loads all settings from `HELIX_MEMORY_*` environment variables with sensible defaults. Covers backend endpoints, fusion engine weights, consolidation settings, circuit breaker thresholds, embedding configuration, and concurrency limits.

### Metrics (`pkg/metrics/metrics.go`)

Prometheus metrics covering all operational aspects:
- Search and add latency histograms (per source, per status)
- Operation counters (search, add, consolidation)
- Provider health gauges
- Fusion pipeline metrics (entry counts, deduplication counts)
- Circuit breaker state tracking
- Active provider count

### 12 Power Features (`pkg/features/`)

| # | Feature | Package | Description |
|---|---------|---------|-------------|
| 1 | Codebase DNA Profiling | `codebase_dna` | Builds memory profiles of coding patterns, preferences, architecture decisions, and tech stack from codebase interactions |
| 2 | Memory-Augmented AI Debate | `debate_memory` | Provides memory-backed context injection for debate agents, enabling debates to leverage historical knowledge and past decisions |
| 3 | Adaptive Context Window | `context_window` | Dynamically manages LLM context windows by selecting the most relevant memories within token limits |
| 4 | Multi-Agent Memory Mesh | `mesh` | Enables multiple AI agents to share memories with scope isolation (private/team/global) and access control |
| 5 | Temporal Reasoning | `temporal` | Bi-temporal queries, timeline construction, edge invalidation, and "what was true at time T?" reasoning via Graphiti |
| 6 | Confidence Scoring and Provenance | `confidence` | Multi-factor confidence scoring (source reliability 0.30, cross-validation 0.25, recency 0.20, access frequency 0.15, coherence 0.10) with full provenance tracking |
| 7 | Self-Improving Quality Loop | `quality_loop` | Continuously monitors memory quality, identifies stale/contradictory/low-confidence entries, and recommends cleanup actions |
| 8 | Memory Snapshots and Rollback | `snapshots` | Point-in-time snapshots of memory state for backup, comparison, and rollback operations |
| 9 | Procedural Memory | `procedural` | Captures learned workflows, debugging strategies, deployment procedures, and "how-to" knowledge |
| 10 | Cross-Project Knowledge Transfer | `cross_project` | Transfers learned patterns, conventions, and domain knowledge between different projects |
| 11 | MCP Bridge | `mcp_bridge` | Exposes memory operations through MCP-compatible endpoints for external tool and agent interaction |
| 12 | Memory-Driven Code Generation | `code_gen` | Leverages stored coding patterns, conventions, and project DNA for context-aware code generation assistance |

## Data Flow

### Search Flow (Read Path)

```
                        Search Request
                             |
                    +--------v--------+
                    |     Router      |
                    | RouteRead()     |
                    +--------+--------+
                             |
              +--------------+--------------+
              |              |              |
         +----v----+   +----v----+   +----v----+    +----------+
         |  Mem0   |   | Cognee  |   |  Letta  |    | Graphiti |
         | Client  |   | Client  |   | Client  |    |  Client  |
         +----+----+   +----+----+   +----+----+    +----+-----+
              |              |              |              |
              |   (parallel via errgroup)   |              |
              +--------------+--------------+--------------+
                             |
                    +--------v---------+
                    |   Fusion Engine   |
                    |                   |
                    | 1. Collect &      |
                    |    Normalize      |
                    | 2. Deduplicate    |
                    |    (cosine >= 0.92)|
                    | 3. Re-Rank        |
                    |    (weighted)     |
                    +--------+---------+
                             |
                    +--------v--------+
                    |  Unified Result  |
                    |  (SearchResult)  |
                    +-----------------+
```

### Write Flow (Write Path)

```
                        Memory Entry
                             |
                    +--------v--------+
                    |     Router      |
                    | ClassifyType()  |
                    | RouteWrite()    |
                    +--------+--------+
                             |
                    +--------v--------+
                    | Primary Backend |
                    |   (by type)     |
                    +--------+--------+
                             |
                   success?--+--failure?
                      |             |
                   return      +----v----+
                               | Fallback|
                               | Backends|
                               +---------+
```

### Consolidation Flow (Background)

```
                   Timer (30m interval)
                             |
                    +--------v--------+
                    | Collect recent  |
                    | from all backends|
                    +--------+--------+
                             |
                    +--------v--------+
                    | Deduplicate     |
                    | by entry ID     |
                    +--------+--------+
                             |
                    +--------v--------+
                    | Cross-reference |
                    | and enrich      |
                    +--------+--------+
                             |
                    +--------v--------+
                    | Update stats    |
                    +-----------------+
```

## Package Structure

```
HelixMemory/
├── pkg/
│   ├── types/                        # Core types and interfaces
│   │   ├── types.go                  #   MemoryEntry, MemoryType, MemorySource,
│   │   │                             #   MemoryProvider, CoreMemoryProvider,
│   │   │                             #   ConsolidationProvider, TemporalProvider
│   │   └── circuit_breaker.go        #   Circuit breaker implementation
│   ├── config/
│   │   └── config.go                 # HELIX_MEMORY_* env var configuration
│   ├── clients/
│   │   ├── mem0/
│   │   │   └── client.go             # Mem0 REST API client
│   │   ├── cognee/
│   │   │   └── client.go             # Cognee ECL pipeline client
│   │   ├── letta/
│   │   │   └── client.go             # Letta agent runtime client
│   │   └── graphiti/
│   │       └── client.go             # Graphiti temporal graph client
│   ├── fusion/
│   │   └── engine.go                 # 3-stage fusion (collect, dedup, rerank)
│   ├── routing/
│   │   └── router.go                 # Intelligent memory routing/classification
│   ├── provider/
│   │   ├── unified.go                # UnifiedMemoryProvider orchestrator
│   │   └── adapter.go                # MemoryStore interface adapter
│   ├── consolidation/
│   │   └── consolidation.go          # Sleep-time compute engine
│   ├── metrics/
│   │   └── metrics.go                # Prometheus metrics
│   ├── infra/
│   │   ├── infra.go                  # InfraProvider interface, endpoints
│   │   └── compose.go                # Docker Compose infrastructure
│   └── features/
│       ├── codebase_dna/dna.go       # Codebase DNA profiling
│       ├── debate_memory/debate.go   # Memory-augmented debate
│       ├── context_window/context.go # Adaptive context window
│       ├── mesh/mesh.go              # Multi-agent memory mesh
│       ├── temporal/temporal.go      # Temporal reasoning
│       ├── confidence/scoring.go     # Confidence scoring & provenance
│       ├── quality_loop/quality.go   # Self-improving quality loop
│       ├── snapshots/snapshots.go    # Memory snapshots & rollback
│       ├── procedural/procedural.go  # Procedural memory
│       ├── cross_project/transfer.go # Cross-project transfer
│       ├── mcp_bridge/bridge.go      # MCP protocol bridge
│       └── code_gen/codegen.go       # Memory-driven code generation
├── docker/
│   └── docker-compose.yml            # Infrastructure: Letta, Mem0, Cognee,
│                                     #   Qdrant, Neo4j, Redis, PostgreSQL
├── tests/                            # Integration, E2E, security, stress,
│                                     #   benchmark, automation tests
├── go.mod                            # Module: digital.vasic.helixmemory
├── CLAUDE.md                         # Development instructions
└── README.md                         # Project documentation
```

## Key Interfaces

| Interface | Package | Methods | Purpose |
|-----------|---------|---------|---------|
| `MemoryProvider` | `types` | Name, Add, Search, Get, Update, Delete, GetHistory, Health | Base contract for all memory backends |
| `CoreMemoryProvider` | `types` | extends MemoryProvider + GetCoreMemory, UpdateCoreMemory | Letta-style editable memory blocks |
| `ConsolidationProvider` | `types` | extends MemoryProvider + TriggerConsolidation, GetConsolidationStatus | Sleep-time compute capability |
| `TemporalProvider` | `types` | extends MemoryProvider + SearchTemporal, GetTimeline, InvalidateAt | Time-aware queries via Graphiti |
| `InfraProvider` | `infra` | Start, Stop, HealthCheck, GetEndpoint, IsRemote | Container infrastructure abstraction |

## Integration with HelixAgent

HelixMemory integrates with HelixAgent through three mechanisms:

1. **MemoryStore Interface** -- The `MemoryStoreAdapter` (`pkg/provider/adapter.go`) implements `digital.vasic.memory/pkg/store.MemoryStore`, making HelixMemory a transparent drop-in replacement. When the HelixMemory submodule is present, HelixAgent automatically uses it instead of the default Memory module. The adapter is activated by default; opt out with `-tags nohelixmemory`.

2. **Internal Adapter** -- HelixAgent's adapter layer (`internal/adapters/`) bridges HelixAgent's internal types to the HelixMemory module. The adapter initializes the UnifiedMemoryProvider, registers all available backend clients, starts the consolidation engine, and exposes the provider through HelixAgent's service layer.

3. **Infrastructure Bridge** -- The `infra` package connects to the `digital.vasic.containers` module for container orchestration of HelixMemory's 7 infrastructure services (Letta :8283, Mem0 :8001, Cognee :8000, Qdrant :6333, Neo4j :7474/:7687, Redis :6379, PostgreSQL :5432). Services are auto-booted as part of HelixAgent's startup sequence via the centralized Containers module adapter.

4. **Debate Integration** -- The `debate_memory` feature provides memory-backed context injection for HelixAgent's AI debate system, enabling debate agents to leverage historical knowledge, past decisions, and learned patterns stored across all four backends.

## Infrastructure Services

| Service | Port | Protocol | Role |
|---------|------|----------|------|
| Letta | 8283 | HTTP | Stateful agent runtime, core memory |
| Mem0 | 8001 | HTTP | Fact extraction, preference management |
| Cognee | 8000 | HTTP | Knowledge graphs, ECL pipelines |
| Graphiti | 8003 | HTTP | Temporal knowledge graph |
| Qdrant | 6333 | HTTP | Vector similarity search |
| Neo4j | 7687 | Bolt | Graph database (used by Cognee/Graphiti) |
| Redis | 6379 | TCP | Caching, session state |
| PostgreSQL | 5432 | TCP | Persistent storage |
