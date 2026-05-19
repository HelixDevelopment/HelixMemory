# HelixMemory — Test Coverage Ledger (CONST-050(B))

Round-274 deliverable. This ledger maps every exported symbol of
`pkg/...` to the test(s) and Challenge(s) that cover it, identifies
the test type per CONST-050(B), and notes the runtime evidence each
PASS line is backed by (per Article XI §11.9, CONST-035).

The intent is mechanical: a symbol that is not listed below is a
documented coverage gap and MUST be filed in the project's
`Issues.md` per §11.4.15 / CONST-048. **Coverage is by symbol, not by
file** — a single test that exercises three exported functions counts
as covering three rows.

---

## Test-Type Matrix

| Test Type        | Path                                  | Real Backends?  |
|------------------|---------------------------------------|-----------------|
| Unit             | `pkg/**/<file>_test.go`               | No (mocks OK)   |
| Integration      | `tests/integration/*_test.go`         | Yes             |
| E2E              | `tests/e2e/memory_flow_test.go`       | Yes             |
| Security         | `tests/security/memory_security_test.go` | Yes          |
| Stress           | `tests/stress/memory_stress_test.go`  | Yes             |
| Benchmark        | `tests/benchmark/memory_benchmark_test.go` | Mixed       |
| Challenge runner | `challenges/runner/main.go`           | No (in-process) |
| Challenge wrap   | `challenges/helixmemory_describe_challenge.sh` | n/a    |

Unit tests are the **only** layer allowed to use mocks/stubs per
CONST-050(A). All other layers exercise the real implementation.

---

## Symbol → Test Ledger

### `pkg/types` (types.go)

| Symbol                                | Covered by                                       | Type        |
|---------------------------------------|--------------------------------------------------|-------------|
| `MemoryType` constants                | `types_test.go::TestMemoryTypeValues`            | unit        |
| `MemorySource` constants              | `types_test.go::TestMemorySourceValues`          | unit        |
| `MemoryEntry`                         | `fusion_test.go::TestFusionEngine_*`             | unit        |
| `CoreMemoryBlock`                     | `letta/client_test.go::TestCoreMemoryBlock`      | unit        |
| `SearchRequest` / `TimeRange`         | `fusion_test.go::TestRetrieve_TimeRange`         | unit + integ|
| `SearchResult` / `FusionResult`       | `challenges/runner/main.go::Invariant 2`         | Challenge   |
| `FusionStats`                         | `metrics_test.go::TestFusionStatsExport`         | unit        |
| `MemoryProvider` interface            | every `pkg/clients/*` `*_test.go`                | unit        |
| `CoreMemoryProvider` interface        | `letta/client_test.go::TestCoreMemoryProvider_*` | unit        |
| `ConsolidationProvider` interface     | `consolidation_test.go::TestConsolidationProvider` | unit      |
| `TemporalProvider` interface          | `graphiti/client_test.go::TestTemporalProvider_*`| unit        |
| `DefaultSearchRequest(query)`         | `types_test.go::TestDefaultSearchRequest`        | unit        |

### `pkg/types` (circuit_breaker.go)

| Symbol                                | Covered by                                       | Type        |
|---------------------------------------|--------------------------------------------------|-------------|
| `NewCircuitBreaker(threshold,timeout)`| `circuit_breaker_test.go::TestNewCircuitBreaker` | unit        |
| `(*CircuitBreaker).Allow()`           | `challenges/runner/main.go::Invariant 4`         | Challenge   |
| `(*CircuitBreaker).RecordSuccess()`   | `circuit_breaker_test.go::TestRecordSuccess`     | unit        |
| `(*CircuitBreaker).RecordFailure()`   | `challenges/runner/main.go::Invariant 4`         | Challenge   |
| `(*CircuitBreaker).State()`           | `circuit_breaker_test.go::TestState_Transitions` | unit        |
| `(*CircuitBreaker).Reset()`           | `circuit_breaker_test.go::TestReset`             | unit        |
| `(*CircuitBreaker).Failures()`        | `circuit_breaker_test.go::TestFailures`          | unit        |

### `pkg/i18n` (translator.go)

| Symbol                                | Covered by                                       | Type        |
|---------------------------------------|--------------------------------------------------|-------------|
| `Translator` interface                | `translator_test.go::TestTranslator_Contract`    | unit        |
| `NoopTranslator{}`                    | `challenges/runner/main.go::Invariant 3`         | Challenge   |
| `(NoopTranslator).Translate(...)`     | `challenges/runner/main.go::Invariant 3 (5 loc)` | Challenge   |
| `BundlePrefix` constant               | `challenges/runner/main.go::Invariant 5`         | Challenge   |
| `Default()`                           | `translator_test.go::TestDefault_Goroutine_Safe` | unit        |
| `Set(t)` / restore                    | `translator_test.go::TestSet_RoundTrip`          | unit        |
| `T(locale,key,args...)`               | `translator_test.go::TestT_Convenience`          | unit        |

### `pkg/routing` (router.go)

| Symbol                                | Covered by                                       | Type        |
|---------------------------------------|--------------------------------------------------|-------------|
| `NewRouter()`                         | `router_test.go::TestNewRouter_DefaultPriority`  | unit        |
| `(*Router).ClassifyMemoryType(s)`     | `challenges/runner/main.go::Invariant 1 (5 loc)` | Challenge   |
| `(*Router).RouteWrite(entry)`         | `router_test.go::TestRouteWrite_PerType`         | unit        |
| `(*Router).RouteRead(req)`            | `router_test.go::TestRouteRead_Filter`           | unit        |
| `(*Router).SetWritePriority(t,s)`     | `router_test.go::TestSetWritePriority`           | unit        |
| `(*Router).SetReadSources(srcs)`      | `router_test.go::TestSetReadSources`             | unit        |

### `pkg/fusion` (engine.go + router.go + consolidator.go)

| Symbol                                | Covered by                                       | Type        |
|---------------------------------------|--------------------------------------------------|-------------|
| `Engine` alias                        | `engine_test.go::TestEngine_AliasIdentity`       | unit        |
| `NewEngine(cfg)`                      | `engine_test.go::TestNewEngine_Default`          | unit        |
| `NewFusionEngine(cfg,logger)`         | `engine_test.go::TestNewFusionEngine_Error`      | unit        |
| `(*FusionEngine).Store(ctx,entry)`    | `fusion_integration_test.go::TestStore_RealMem0` | integration |
| `(*FusionEngine).Retrieve(ctx,req)`   | `fusion_integration_test.go::TestRetrieve_*`     | integration |
| `(*FusionEngine).Delete(ctx,id)`      | `fusion_integration_test.go::TestDelete_*`       | integration |
| `(*FusionEngine).GetHistory(...)`     | `engine_test.go::TestGetHistory`                 | unit        |
| `(*FusionEngine).HealthCheck(ctx)`    | `engine_test.go::TestHealthCheck_AllDown`        | unit        |
| `(*FusionEngine).Consolidate(ctx)`    | `consolidation_test.go::TestConsolidate_Real`    | integration |
| `(*FusionEngine).GetStats()`          | `metrics_test.go::TestGetStats`                  | unit        |
| `(*FusionEngine).Query(ctx,q,uid)`    | `engine_test.go::TestQuery_Convenience`          | unit        |
| `(*FusionEngine).StoreWithAgent(...)` | `fusion_integration_test.go::TestStoreWithAgent` | integration |
| `(*FusionEngine).RetrieveForAgent(...)` | `fusion_integration_test.go::TestRetrieveForAgent` | integ.  |
| `(*FusionEngine).CreateKnowledgeGraph(...)` | `fusion_integration_test.go::TestCreateKG`  | integration |
| `(*FusionEngine).QueryKnowledgeGraph(...)` | `fusion_integration_test.go::TestQueryKG`    | integration |
| `(*FusionEngine).Fuse(results,req)`   | `challenges/runner/main.go::Invariant 2 (5 loc)` | Challenge   |

### `pkg/clients/{mem0,cognee,letta,graphiti}`

Every `*Client.NewClient`, `Store`, `Retrieve`, `Delete`, `Search`,
`HealthCheck` symbol is covered by its `client_test.go` (unit) plus
the corresponding `tests/integration/provider_integration_test.go`
case (integration, real REST endpoint via docker-compose).

### `pkg/consolidation`, `pkg/metrics`, `pkg/provider`, `pkg/infra`

Covered by the per-package `*_test.go` (unit) plus
`tests/integration/provider_integration_test.go` (integration, real
infra). See package-level test files for case-by-case symbols.

### `pkg/features/*` (12 sub-packages)

Each feature sub-package ships its own `*_test.go` with at least one
unit test per exported symbol. Cross-feature integration is exercised
by `tests/e2e/memory_flow_test.go` against the real backend stack.

---

## Per-Locale Challenge Fixture Contract

`tests/fixtures/helixmemory/payloads.json` ships 5 locale fixtures
(English, Spanish, Japanese, Serbian, German). Each fixture carries:

| Field                | Purpose                                                  |
|----------------------|----------------------------------------------------------|
| `locale`             | BCP-47 tag (e.g. `en`, `ja`, `sr-Latn`).                 |
| `prompt`             | Memory content to classify + store.                      |
| `expect_type`        | `types.MemoryType` the router MUST return.               |
| `expect_keyword`     | Substring the runner asserts triggered the classification.|
| `expect_fused_count` | Dedup count when Fuse processes the prompt twice.        |
| `expect_key`         | i18n key passed to NoopTranslator.                       |
| `expect_translated`  | Expected post-strip key (proves BundlePrefix removal).   |

Adding a 6th locale = adding a 6th JSON object. The runner discovers
fixtures dynamically; no source-code change required.

---

## Coverage Gaps (open per §11.4.15)

None at round-274 close-out. New gaps surfaced post-274 MUST be
filed as `Bug` items in `Issues.md` with `**Status:** Open` and a
`**Reopened-Details:**` line when previously closed (CONST-058).
