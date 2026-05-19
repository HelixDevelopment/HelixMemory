# HelixMemory

**Unified Cognitive Memory Engine** — Fusing Mem0, Cognee, Letta, and
Graphiti into a single orchestrated memory system for AI applications.

Module path: `digital.vasic.helixmemory` (Go 1.25+).

---

## What is HelixMemory?

HelixMemory is a Go SDK that combines four best-in-class memory systems
into a unified cognitive memory engine — without re-implementing any of
them. Each backend handles what it does best; a fusion engine combines
their results into a single, deduplicated, re-ranked memory experience.

- **Mem0** — Dynamic fact extraction + preference management (26%+
  accuracy over baseline).
- **Cognee** — Semantic knowledge graphs via ECL pipelines (38+ data
  source connectors).
- **Letta** — Stateful agent runtime with sleep-time compute and
  editable memory blocks.
- **Graphiti** — Temporal knowledge graph with bi-temporal data
  modeling.

## Key Features

- **Parallel search** across all backends with automatic fusion.
- **3-stage fusion**: Collection → Deduplication → Cross-Source
  Re-Ranking.
- **Intelligent routing** — memories classified by content keywords
  and routed to the optimal backend (`pkg/routing/router.go`).
- **Graceful degradation** — if a backend is down, the rest continue
  serving (circuit breakers in `pkg/types/circuit_breaker.go`).
- **Sleep-time compute** — consolidation during idle periods
  (`pkg/consolidation/`).
- **MemoryStore interface** — drop-in replacement for
  `digital.vasic.memory` (`pkg/provider/adapter.go`).
- **Prometheus metrics** for full observability (`pkg/metrics/`).
- **i18n translator seam** — CONST-046-clean string surface ready
  for any future user-facing CLI/REST layer (`pkg/i18n/`).

## Quick Start

```bash
# 1. Run the unit + race-detector test suite (no infrastructure required):
go test -race -count=1 ./pkg/...

# 2. Run the round-274 Challenge runner (deterministic, in-process,
#    exercises Router + FusionEngine.Fuse + Translator across 5 locales):
go run ./challenges/runner/

# 3. Run the paired-mutation Challenge wrapper:
./challenges/helixmemory_describe_challenge.sh normal   # exits 0
./challenges/helixmemory_describe_challenge.sh mutate   # exits 99

# 4. Bring up the real backend stack (PostgreSQL + Neo4j + Redis +
#    Mem0/Cognee/Letta REST mocks) for integration tests:
make infra-start
make test-integration
make infra-stop
```

## Anti-Bluff Guarantees (round-274)

Round-274 adds an in-process Challenge runner + paired-mutation wrapper
that mirror the canonical pattern established for HelixSpecifier
(round-273) and HelixCognitiveCore (round-220). The runner exercises
**real production code** — no mocks, no stubs — and asserts a closed
list of invariants:

1. **routing.Router.ClassifyMemoryType** returns the documented
   `types.MemoryType` for each locale fixture's `expect_type` keyword
   (the lexicon really fires, the test is not a tautology).
2. **fusion.FusionEngine.Fuse** with two overlapping `*SearchResult`
   inputs produces a deduplicated output whose `Entries` count equals
   the fixture's `expect_fused_count` (proves the dedup stage actually
   runs, not just `len(a)+len(b)`).
3. **i18n.NoopTranslator.Translate** returns the key with the
   `helixmemory_` namespace prefix stripped — the documented anti-bluff
   contract (missing translations surface as the readable key, never
   silent empty strings).
4. **types.NewCircuitBreaker / Allow / RecordFailure / State** behave
   per the documented state machine (closed → open after threshold
   failures; half-open after timeout; back to closed on success).
5. **i18n.BundlePrefix** is exactly `helixmemory_` — cross-submodule
   uniqueness invariant required by the future bundle-merger
   (changing this value silently breaks the §CONST-046 audit gate).

Every PASS line in the runner's output is backed by a runtime
invariant — **no metadata-only PASS, no absence-of-error PASS, no
grep-based PASS** (Article XI §11.9, CONST-035).

### Paired-mutation contract (CONST-050(A), §1.1)

`challenges/helixmemory_describe_challenge.sh mutate` sets
`HELIXMEMORY_MUTATE_RUNNER=1`, which flips invariant 3 inside the
runner (treats a successful key-roundtrip as FAIL instead of PASS).
The wrapper then asserts the runner exits non-zero — proving the
runner actually checks what it claims. The wrapper rewrites the
mutation-detected exit to `99`. If the runner exits 0 under
mutation, the wrapper exits 1 (a CONST-050(A) violation).

## Architecture

```
┌─────────────────────────────────────────────┐
│       UnifiedMemoryProvider                 │
│                                             │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌────────┐    │
│  │ Mem0 │ │Cognee│ │Letta │ │Graphiti│    │
│  └──┬───┘ └──┬───┘ └──┬───┘ └───┬────┘    │
│     │        │        │         │          │
│  ┌──▼────────▼────────▼─────────▼──┐       │
│  │     routing.Router               │       │
│  │  ClassifyMemoryType / RouteWrite │       │
│  │  RouteRead                       │       │
│  └──┬───────────────────────────────┘       │
│     │                                       │
│  ┌──▼───────────────────────────────┐       │
│  │     fusion.FusionEngine          │       │
│  │  Store / Retrieve / Delete /     │       │
│  │  Consolidate / Fuse              │       │
│  └──┬───────────────────────────────┘       │
│     │                                       │
│  ┌──▼─────────┬───────────┬──────────┐     │
│  │  Collect   │ Dedupe    │ Re-rank  │     │
│  └────────────┴───────────┴──────────┘     │
└─────────────────────────────────────────────┘
```

## Package Map

| Package                          | Responsibility                                       |
|----------------------------------|------------------------------------------------------|
| `pkg/types`                      | `MemoryEntry`, `SearchRequest`, `MemoryProvider`     |
| `pkg/types` (circuit_breaker.go) | Circuit-breaker state machine for backend failover   |
| `pkg/routing`                    | Keyword-driven memory-type classification + routing  |
| `pkg/fusion`                     | 3-stage fusion engine (Collect → Dedupe → Rerank)    |
| `pkg/consolidation`              | Sleep-time consolidation worker                      |
| `pkg/i18n`                       | Translator seam (CONST-046 compliance, NoopFallback) |
| `pkg/metrics`                    | Prometheus counters / histograms / gauges            |
| `pkg/provider`                   | `MemoryStore` adapter for `digital.vasic.memory`     |
| `pkg/clients/{mem0,cognee,letta,graphiti}` | REST/SDK clients per backend                |
| `pkg/infra`                      | docker-compose lifecycle for integration tests       |
| `pkg/features/*`                 | Cross-feature surfaces (mesh, snapshots, temporal, …)|

## Test Coverage (CONST-050(B))

See [`docs/test-coverage.md`](docs/test-coverage.md) for the full
symbol→test ledger covering every exported function across `pkg/...`,
the test-type matrix (unit / integration / E2E / security / stress /
benchmark / Challenge), and the per-locale Challenge fixture
contract.

## Integration with HelixCode

HelixMemory implements `digital.vasic.memory/pkg/store.MemoryStore`,
making it a drop-in replacement. When the HelixMemory submodule is
present, HelixCode automatically uses it instead of the default
Memory module. The replacement is consumer-side only — HelixMemory
itself does not reach back into HelixCode (CONST-051(B) decoupling).

## License

Proprietary — HelixDevelopment Organization
