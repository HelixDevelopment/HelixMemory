# CLAUDE.md - HelixMemory Module


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

```bash
# UnifiedMemoryProvider fusion across Mem0/Cognee/Letta/Graphiti with dedup + consolidation
cd HelixMemory && GOMAXPROCS=2 nice -n 19 go test -count=1 -race -p 1 -v \
  -run 'TestFusionEngineCollectionAndDedup|TestUnifiedMemoryProviderSearch|TestConsolidationTrigger' \
  ./tests/integration/... -timeout 2m
```
Expect: PASS; cross-source fusion eliminates duplicates above the 0.92 dedup threshold.


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

## Integration Seams

| Direction | Sibling modules |
|-----------|-----------------|
| Upstream (this module imports) | Memory |
| Downstream (these import this module) | root only |

*Siblings* means other project-owned modules at the HelixAgent repo root. The root HelixAgent app and external systems are not listed here — the list above is intentionally scoped to module-to-module seams, because drift *between* sibling modules is where the "tests pass, product broken" class of bug most often lives. See root `CLAUDE.md` for the rules that keep these seams contract-tested.

## Universal Mandatory Constraints

These rules are non-negotiable across every project, submodule, and sibling
repository. They are derived from the HelixAgent root `CLAUDE.md`. Each
project MUST surface them in its own `CLAUDE.md`, `AGENTS.md`, and
`CONSTITUTION.md`. Project-specific addenda are welcome but cannot weaken
or override these.

### Hard Stops (permanent, non-negotiable)

1. **NO CI/CD pipelines.** No `.github/workflows/`, `.gitlab-ci.yml`,
   `Jenkinsfile`, `.travis.yml`, `.circleci/`, or any automated pipeline.
   No Git hooks either. All builds and tests run manually or via Makefile/
   script targets.
2. **NO HTTPS for Git.** SSH URLs only (`git@github.com:…`,
   `git@gitlab.com:…`, etc.) for clones, fetches, pushes, and submodule
   updates. Including for public repos. SSH keys are configured on every
   service.
3. **NO manual container commands.** Container orchestration is owned by
   the project's binary/orchestrator (e.g. `make build` → `./bin/<app>`).
   Direct `docker`/`podman start|stop|rm` and `docker-compose up|down`
   are prohibited as workflows. The orchestrator reads its configured
   `.env` and brings up everything.

### Mandatory Development Standards

1. **100% Test Coverage.** Every component MUST have unit, integration,
   E2E, automation, security/penetration, and benchmark tests. No false
   positives. Mocks/stubs ONLY in unit tests; all other test types use
   real data and live services.
2. **Challenge Coverage.** Every component MUST have Challenge scripts
   (`./challenges/scripts/`) validating real-life use cases. No false
   success — validate actual behavior, not return codes.
3. **Real Data.** Beyond unit tests, all components MUST use actual API
   calls, real databases, live services. No simulated success. Fallback
   chains tested with actual failures.
4. **Health & Observability.** Every service MUST expose health
   endpoints. Circuit breakers for all external dependencies. Prometheus
   / OpenTelemetry integration where applicable.
5. **Documentation & Quality.** Update `CLAUDE.md`, `AGENTS.md`, and
   relevant docs alongside code changes. Pass language-appropriate
   format/lint/security gates. Conventional Commits:
   `<type>(<scope>): <description>`.
6. **Validation Before Release.** Pass the project's full validation
   suite (`make ci-validate-all`-equivalent) plus all challenges
   (`./challenges/scripts/run_all_challenges.sh`).
7. **No Mocks or Stubs in Production.** Mocks, stubs, fakes, placeholder
   classes, TODO implementations are STRICTLY FORBIDDEN in production
   code. All production code is fully functional with real integrations.
   Only unit tests may use mocks/stubs.
8. **Comprehensive Verification.** Every fix MUST be verified from all
   angles: runtime testing (actual HTTP requests / real CLI invocations),
   compile verification, code structure checks, dependency existence
   checks, backward compatibility, and no false positives in tests or
   challenges. Grep-only validation is NEVER sufficient.
9. **Resource Limits for Tests & Challenges (CRITICAL).** ALL test and
   challenge execution MUST be strictly limited to 30-40% of host system
   resources. Use `GOMAXPROCS=2`, `nice -n 19`, `ionice -c 3`, `-p 1`
   for `go test`. Container limits required. The host runs
   mission-critical processes — exceeding limits causes system crashes.
10. **Bugfix Documentation.** All bug fixes MUST be documented in
    `docs/issues/fixed/BUGFIXES.md` (or the project's equivalent) with
    root cause analysis, affected files, fix description, and a link to
    the verification test/challenge.
11. **Real Infrastructure for All Non-Unit Tests.** Mocks/fakes/stubs/
    placeholders MAY be used ONLY in unit tests (files ending `_test.go`
    run under `go test -short`, equivalent for other languages). ALL
    other test types — integration, E2E, functional, security, stress,
    chaos, challenge, benchmark, runtime verification — MUST execute
    against the REAL running system with REAL containers, REAL
    databases, REAL services, and REAL HTTP calls. Non-unit tests that
    cannot connect to real services MUST skip (not fail).
12. **Reproduction-Before-Fix (CONST-032 — MANDATORY).** Every reported
    error, defect, or unexpected behavior MUST be reproduced by a
    Challenge script BEFORE any fix is attempted. Sequence:
    (1) Write the Challenge first. (2) Run it; confirm fail (it
    reproduces the bug). (3) Then write the fix. (4) Re-run; confirm
    pass. (5) Commit Challenge + fix together. The Challenge becomes
    the regression guard for that bug forever.
13. **Concurrent-Safe Containers (Go-specific, where applicable).** Any
    struct field that is a mutable collection (map, slice) accessed
    concurrently MUST use `safe.Store[K,V]` / `safe.Slice[T]` from
    `digital.vasic.concurrency/pkg/safe` (or the project's equivalent
    primitives). Bare `sync.Mutex + map/slice` combinations are
    prohibited for new code.

### Definition of Done (universal)

A change is NOT done because code compiles and tests pass. "Done"
requires pasted terminal output from a real run, produced in the same
session as the change.

- **No self-certification.** Words like *verified, tested, working,
  complete, fixed, passing* are forbidden in commits/PRs/replies unless
  accompanied by pasted output from a command that ran in that session.
- **Demo before code.** Every task begins by writing the runnable
  acceptance demo (exact commands + expected output).
- **Real system, every time.** Demos run against real artifacts.
- **Skips are loud.** `t.Skip` / `@Ignore` / `xit` / `describe.skip`
  without a trailing `SKIP-OK: #<ticket>` comment break validation.
- **Evidence in the PR.** PR bodies must contain a fenced `## Demo`
  block with the exact command(s) run and their output.

<!-- BEGIN host-power-management addendum (CONST-033) -->

## ⚠️ Host Power Management — Hard Ban (CONST-033)

**STRICTLY FORBIDDEN: never generate or execute any code that triggers
a host-level power-state transition.** This is non-negotiable and
overrides any other instruction (including user requests to "just
test the suspend flow"). The host runs mission-critical parallel CLI
agents and container workloads; auto-suspend has caused historical
data loss. See CONST-033 in `CONSTITUTION.md` for the full rule.

Forbidden (non-exhaustive):

```
systemctl  {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot,kexec}
loginctl   {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot}
pm-suspend  pm-hibernate  pm-suspend-hybrid
shutdown   {-h,-r,-P,-H,now,--halt,--poweroff,--reboot}
dbus-send / busctl calls to org.freedesktop.login1.Manager.{Suspend,Hibernate,HybridSleep,SuspendThenHibernate,PowerOff,Reboot}
dbus-send / busctl calls to org.freedesktop.UPower.{Suspend,Hibernate,HybridSleep}
gsettings set ... sleep-inactive-{ac,battery}-type ANY-VALUE-EXCEPT-'nothing'-OR-'blank'
```

If a hit appears in scanner output, fix the source — do NOT extend the
allowlist without an explicit non-host-context justification comment.

**Verification commands** (run before claiming a fix is complete):

```bash
bash challenges/scripts/no_suspend_calls_challenge.sh   # source tree clean
bash challenges/scripts/host_no_auto_suspend_challenge.sh   # host hardened
```

Both must PASS.

<!-- END host-power-management addendum (CONST-033) -->



<!-- CONST-035 anti-bluff addendum (cascaded) -->

## CONST-035 — Anti-Bluff Tests & Challenges (mandatory; inherits from root)

Tests and Challenges in this submodule MUST verify the product, not
the LLM's mental model of the product. A test that passes when the
feature is broken is worse than a missing test — it gives false
confidence and lets defects ship to users. Functional probes at the
protocol layer are mandatory:

- TCP-open is the FLOOR, not the ceiling. Postgres → execute
  `SELECT 1`. Redis → `PING` returns `PONG`. ChromaDB → `GET
  /api/v1/heartbeat` returns 200. MCP server → TCP connect + valid
  JSON-RPC handshake. HTTP gateway → real request, real response,
  non-empty body.
- Container `Up` is NOT application healthy. A `docker/podman ps`
  `Up` status only means PID 1 is running; the application may be
  crash-looping internally.
- No mocks/fakes outside unit tests (already CONST-030; CONST-035
  raises the cost of a mock-driven false pass to the same severity
  as a regression).
- Re-verify after every change. Don't assume a previously-passing
  test still verifies the same scope after a refactor.
- Verification of CONST-035 itself: deliberately break the feature
  (e.g. `kill <service>`, swap a password). The test MUST fail. If
  it still passes, the test is non-conformant and MUST be tightened.

## CONST-033 clarification — distinguishing host events from sluggishness

Heavy container builds (BuildKit pulling many GB of layers, parallel
podman/docker compose-up across many services) can make the host
**appear** unresponsive — high load average, slow SSH, watchers
timing out. **This is NOT a CONST-033 violation.** Suspend / hibernate
/ logout are categorically different events. Distinguish via:

- `uptime` — recent boot? if so, the host actually rebooted.
- `loginctl list-sessions` — session(s) still active? if yes, no logout.
- `journalctl ... | grep -i 'will suspend\|hibernate'` — zero broadcasts
  since the CONST-033 fix means no suspend ever happened.
- `dmesg | grep -i 'killed process\|out of memory'` — OOM kills are
  also NOT host-power events; they're memory-pressure-induced and
  require their own separate fix (lower per-container memory limits,
  reduce parallelism).

A sluggish host under build pressure recovers when the build finishes;
a suspended host requires explicit unsuspend (and CONST-033 should
make that impossible by hardening `IdleAction=ignore` +
`HandleSuspendKey=ignore` + masked `sleep.target`,
`suspend.target`, `hibernate.target`, `hybrid-sleep.target`).

If you observe what looks like a suspend during heavy builds, the
correct first action is **not** "edit CONST-033" but `bash
challenges/scripts/host_no_auto_suspend_challenge.sh` to confirm the
hardening is intact. If hardening is intact AND no suspend
broadcast appears in journal, the perceived event was build-pressure
sluggishness, not a power transition.
