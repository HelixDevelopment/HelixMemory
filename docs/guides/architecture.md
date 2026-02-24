# HelixMemory Architecture

This document provides a deep dive into the architecture of HelixMemory, the unified cognitive memory engine that fuses Mem0, Cognee, Letta, and Graphiti into a single orchestrated system.

## System Overview

HelixMemory acts as a "nervous system" that coordinates four specialized memory backends, each with a distinct cognitive role. The UnifiedMemoryProvider is the central orchestrator that routes writes to the appropriate backend, performs parallel searches across all backends, and fuses results through a 3-stage pipeline.

```
                        +-------------------------------------------+
                        |        UnifiedMemoryProvider               |
                        |        (The Nervous System)                |
                        |                                           |
       Add/Update  ---->|   +----------+        +-----------+       |
                        |   |  Router  |------->| Write to  |       |
                        |   | (route   |        | Backend   |       |
                        |   |  by type)|        +-----------+       |
                        |   +----------+             |              |
                        |                     Fallback on failure   |
                        |                                           |
       Search -------->|   +------ Parallel Search ------+         |
                        |   |                             |         |
                        |   v         v        v          v         |
                        |  Mem0    Cognee    Letta    Graphiti      |
                        |   |         |        |          |         |
                        |   v         v        v          v         |
                        |   +--- Fusion Engine ----------+         |
                        |   | Stage 1: Collect & Normalize |        |
                        |   | Stage 2: Deduplicate         |        |
                        |   | Stage 3: Re-Rank             |        |
                        |   +-----------------------------+         |
                        |              |                            |
       <--- Results ----|    Ranked MemoryEntries                   |
                        +-------------------------------------------+
```

## Backend Roles

Each backend specializes in a cognitive function:

| Backend  | Role       | Analogy  | Specialization                    | Trust Score |
|----------|------------|----------|-----------------------------------|-------------|
| Letta    | Brain      | Cortex   | Stateful agent, core memory blocks, sleep-time compute | 0.95 |
| Mem0     | Notebook   | Notes    | Dynamic fact extraction, preferences, 26%+ accuracy over baseline | 0.85 |
| Cognee   | Library    | Archive  | Semantic knowledge graphs via ECL pipeline, entity relations | 0.80 |
| Graphiti | Timeline   | Calendar | Temporal knowledge graph, bi-temporal queries, edge invalidation | 0.85 |

## Data Flow: Write Path

When a memory is added via `UnifiedMemoryProvider.Add()`, it flows through the router to a single backend:

```
  MemoryEntry
      |
      v
  +--------+     Classify content     +-----------+
  | Router |------------------------->| Memory    |
  | Route  |     by MemoryType        | Type      |
  | Write  |                          +-----------+
  +--------+                               |
      |                                    v
      |   +----------------------------------------------+
      |   | Type-to-Backend Mapping:                     |
      |   |   fact       --> Mem0                        |
      |   |   graph      --> Cognee                      |
      |   |   core       --> Letta                       |
      |   |   temporal   --> Graphiti                     |
      |   |   episodic   --> Letta                       |
      |   |   procedural --> Cognee                      |
      |   +----------------------------------------------+
      |
      v
  Target Backend
      |
      +--- Success --> Done
      |
      +--- Failure --> Try fallback backends in order
                           |
                           +--- Any success --> Done
                           +--- All fail --> Return error
```

### Router Classification

If a `MemoryEntry` does not have a type set, the router classifies it by analyzing the content for keyword patterns:

- Temporal keywords ("yesterday", "ago", "timeline") --> `temporal`
- Procedural keywords ("how to", "step by step", "workflow") --> `procedural`
- Graph keywords ("relates to", "depends on", "implements") --> `graph`
- Core keywords ("I am", "my name", "I prefer") --> `core`
- Episodic keywords ("conversation", "discussed", "meeting") --> `episodic`
- Default --> `fact`

### Write Fallback

If the primary backend fails, the provider iterates through all other registered backends until one succeeds. This ensures writes are never lost due to a single backend outage.

## Data Flow: Read Path

When a search is performed via `UnifiedMemoryProvider.Search()`, it fans out to all backends in parallel:

```
  SearchRequest
      |
      v
  +--------+     Determine which     +------------------+
  | Router |     sources to query     | Source Selection  |
  | Route  |------------------------>| - Explicit filter |
  | Read   |                         | - Type-based      |
  +--------+                         | - All (default)   |
      |                              +------------------+
      v
  +------- errgroup (parallel) --------+
  |                                     |
  v         v          v          v     |
  Mem0    Cognee     Letta    Graphiti  |
  |         |          |          |     |
  v         v          v          v     |
  Result1  Result2   Result3   Result4  |
  +-------------------------------------+
      |
      | (individual failures logged, not propagated)
      v
  +------------------------------------------+
  | Fusion Engine                             |
  |                                           |
  | Stage 1: Collect & Normalize              |
  |   - Gather entries from all results       |
  |   - Normalize relevance scores to [0, 1]  |
  |                                           |
  | Stage 2: Deduplicate                      |
  |   - Compare each pair of entries          |
  |   - Cosine similarity on embeddings       |
  |   - Jaccard similarity fallback on text   |
  |   - Threshold: >= 0.92                    |
  |   - Keep entry with higher confidence     |
  |                                           |
  | Stage 3: Cross-Source Re-Rank             |
  |   - Apply weighted scoring formula        |
  |   - Sort by final score descending        |
  |   - Apply TopK limit                      |
  +------------------------------------------+
      |
      v
  SearchResult {
      Entries:  ranked and deduplicated
      Total:    count of results
      Duration: aggregate search time
      Sources:  which backends responded
  }
```

### Graceful Degradation

Errors from individual backends during search are silently absorbed. If Cognee is down but Mem0 and Letta are healthy, the search still returns results from the available backends. This means:

- A search with 4 backends, 2 responding, still returns fused results from those 2.
- Only if zero backends respond does the system return an empty result set.
- Health status is tracked separately and does not block search operations.

## Fusion Pipeline

The fusion engine is the core differentiator of HelixMemory. It implements three stages that transform raw multi-source results into a single, high-quality ranked list.

### Stage 1: Collection and Normalization

All entries from all backend results are gathered into a single list. Relevance scores are normalized to the `[0, 1]` range using max-normalization:

```
normalized_relevance = entry.relevance / max(all_relevances)
```

This ensures that entries from different backends (which may use different scoring scales) are comparable.

### Stage 2: Deduplication

Entries that are near-duplicates are identified and merged. The similarity metric depends on available data:

**With embeddings (preferred):**
```
similarity = cosine_similarity(embedding_a, embedding_b)

            dot(a, b)
         = -----------
           ||a|| * ||b||
```

**Without embeddings (fallback):**
```
similarity = jaccard_similarity(tokens_a, tokens_b)

            |tokens_a INTERSECT tokens_b|
         = -------------------------------
            |tokens_a UNION tokens_b|
```

If `similarity >= 0.92` (configurable via `HELIX_MEMORY_FUSION_DEDUP_THRESHOLD`), the entries are considered duplicates. The entry with higher confidence is kept, and the other is discarded.

### Stage 3: Cross-Source Re-Ranking

Each entry receives a composite score using the weighted formula:

```
final_score = relevance * 0.40
            + recency   * 0.25
            + source    * 0.20
            + type      * 0.15
```

**Relevance (40%):** The normalized relevance score from Stage 1.

**Recency (25%):** Exponential decay with a half-life of approximately 7 days:

```
recency = exp(-0.00413 * hours_since_creation)
```

A memory created 1 hour ago scores ~0.996. A memory created 7 days ago scores ~0.50. A memory created 30 days ago scores ~0.04.

**Source Trust (20%):** Hardcoded trust scores for each backend:

| Source   | Trust Score |
|----------|-------------|
| Letta    | 0.95        |
| Mem0     | 0.85        |
| Graphiti | 0.85        |
| Cognee   | 0.80        |
| Fusion   | 0.90        |

**Type Relevance (15%):** If the search request specifies type filters, matching types score 1.0 and non-matching types score 0.3. Otherwise, default type scores apply:

| Type       | Default Score |
|------------|---------------|
| core       | 0.90          |
| fact       | 0.85          |
| procedural | 0.85          |
| graph      | 0.80          |
| temporal   | 0.75          |
| episodic   | 0.70          |

Entries are sorted by final score in descending order, then trimmed to the requested TopK.

## Circuit Breaker Pattern

Every backend client wraps its HTTP calls in a circuit breaker (`types.CircuitBreaker`). The circuit breaker has three states:

```
  CLOSED -----(failures >= threshold)-----> OPEN
    ^                                         |
    |                                    (timeout expires)
    |                                         |
    +---- (2 consecutive successes) ---- HALF-OPEN
```

**Configuration:**
- `threshold`: 5 consecutive failures before opening (configurable via `HELIX_MEMORY_CIRCUIT_BREAKER_THRESHOLD`)
- `timeout`: 30 seconds before trying again (configurable via `HELIX_MEMORY_CIRCUIT_BREAKER_TIMEOUT`)
- `half_open_max`: 2 consecutive successes required to close the circuit

**Behavior:**
- CLOSED: All requests pass through. Failures are counted.
- OPEN: All requests immediately fail with "circuit breaker open". No network calls are made.
- HALF-OPEN: A limited number of requests pass through. If they succeed (2 in a row), the circuit closes. If any fail, it reopens.

This prevents cascading failures when a backend goes down and allows automatic recovery when it comes back.

## Graceful Degradation Strategy

HelixMemory is designed to operate with any subset of backends available:

| Scenario                        | Behavior                                              |
|---------------------------------|-------------------------------------------------------|
| All 4 backends healthy          | Full functionality, richest results                   |
| 3 of 4 backends healthy         | Nearly full functionality, slightly reduced coverage   |
| Only Mem0 healthy               | Fact-based memory only, no graphs/temporal/core        |
| Only Letta healthy              | Core memory and agent-based storage only               |
| No backends healthy             | Health check fails; searches return empty results      |
| Backend recovers after outage   | Circuit breaker transitions to half-open, then closed  |

Write operations follow the same pattern: if the target backend fails, the provider tries all remaining backends as fallbacks.

## Memory Types and When Each Is Used

| Type         | Primary Backend | Use Case                                           |
|--------------|----------------|----------------------------------------------------|
| `fact`       | Mem0           | User preferences, extracted facts, declarative knowledge |
| `graph`      | Cognee         | Entity relationships, dependency graphs, ontologies |
| `core`       | Letta          | Agent persona, user profile, working context        |
| `temporal`   | Graphiti       | Time-stamped events, versioned truths, changelogs   |
| `episodic`   | Letta          | Conversation summaries, session events, meetings    |
| `procedural` | Cognee         | Workflows, debugging strategies, deployment steps   |

## Consolidation (Sleep-Time Compute)

The consolidation engine runs periodically (default: every 30 minutes) to perform background memory optimization:

1. **Collect**: Fetch recent memories from all providers (batch size: 100).
2. **Deduplicate**: Identify and remove exact ID duplicates across backends.
3. **Enrich**: Add consolidation metadata (timestamp, source count) to all entries.

The consolidation loop is controlled by:
- `HELIX_MEMORY_CONSOLIDATION_ENABLED`: Enable/disable (default: `true`)
- `HELIX_MEMORY_CONSOLIDATION_INTERVAL`: Run frequency (default: `30m`)

Statistics are tracked per run (memories processed, deduplicated, consolidated, errors) and exposed via `GetConsolidationStatus()`.

## Concurrency Model

HelixMemory uses Go's concurrency primitives throughout:

- **Provider Map**: Protected by `sync.RWMutex` for concurrent read access during searches and exclusive write access during provider registration.
- **Parallel Search**: Uses `golang.org/x/sync/errgroup` with configurable concurrency limit (`MaxConcurrentQueries`, default: 4).
- **Per-Request Timeout**: Each backend search gets its own `context.WithTimeout` derived from the errgroup context.
- **Circuit Breakers**: Thread-safe with internal `sync.RWMutex`.
- **Consolidation Engine**: Runs in a dedicated goroutine with `sync.RWMutex` for state protection.

## MemoryStoreAdapter Bridge

The `MemoryStoreAdapter` bridges HelixMemory to the `digital.vasic.memory` module's `MemoryStore` interface. This allows HelixMemory to be used as a drop-in replacement anywhere the standard Memory module is expected.

```
  digital.vasic.memory                    digital.vasic.helixmemory
  +-------------------+                   +-------------------------+
  | MemoryStore       |                   | UnifiedMemoryProvider   |
  | interface         | <-- adapter ----> | (4 backends + fusion)   |
  |                   |                   |                         |
  | Add(Memory)       |   translates to   | Add(MemoryEntry)        |
  | Search(query)     |   translates to   | Search(SearchRequest)   |
  | Get(id)           |   translates to   | Get(id)                 |
  | Update(Memory)    |   translates to   | Update(MemoryEntry)     |
  | Delete(id)        |   translates to   | Delete(id)              |
  | List(scope)       |   translates to   | Search(with filter)     |
  +-------------------+                   +-------------------------+
```

Type conversion preserves metadata bidirectionally. The adapter stores `helix_source`, `helix_type`, and `helix_confidence` in the Memory metadata so that HelixMemory-specific information survives the round-trip.

## Prometheus Metrics

When metrics are enabled (`HELIX_MEMORY_ENABLE_METRICS=true`), the following are exported under the `helixmemory_` namespace:

| Metric                                     | Type      | Labels              |
|--------------------------------------------|-----------|---------------------|
| `helixmemory_search_latency_seconds`       | Histogram | source, status      |
| `helixmemory_search_total`                 | Counter   | source, status      |
| `helixmemory_add_total`                    | Counter   | source, type, status|
| `helixmemory_add_latency_seconds`          | Histogram | source, status      |
| `helixmemory_provider_healthy`             | Gauge     | source              |
| `helixmemory_fusion_entries_count`         | Histogram | (none)              |
| `helixmemory_fusion_deduplicated_total`    | Counter   | (none)              |
| `helixmemory_consolidation_runs_total`     | Counter   | status              |
| `helixmemory_consolidation_duration_seconds` | Histogram | (none)            |
| `helixmemory_circuit_breaker_state`        | Gauge     | source              |
| `helixmemory_active_providers`             | Gauge     | (none)              |
