# AGENTS.md - HelixMemory

## Agent Guidelines

### Project Context
HelixMemory is a proprietary unified cognitive memory engine fusing Mem0, Cognee, Letta, and Graphiti. Module: `digital.vasic.helixmemory`.

### Architecture
- **UnifiedMemoryProvider**: Orchestrates all backends with parallel search and fusion
- **Fusion Engine**: 3-stage pipeline (Collection → Deduplication → Re-Ranking)
- **Router**: Classifies memory types and routes to appropriate backends
- **Circuit Breakers**: Fault tolerance for all backend connections
- **MemoryStoreAdapter**: Implements `digital.vasic.memory` interface

### Key Files
- `pkg/types/types.go` — Core types (MemoryEntry, MemoryType, interfaces)
- `pkg/types/circuit_breaker.go` — Circuit breaker implementation
- `pkg/config/config.go` — Configuration from environment
- `pkg/clients/mem0/client.go` — Mem0 REST API client
- `pkg/clients/cognee/client.go` — Cognee ECL pipeline client
- `pkg/clients/letta/client.go` — Letta agent runtime client
- `pkg/clients/graphiti/client.go` — Graphiti temporal graph client
- `pkg/fusion/engine.go` — 3-stage fusion engine
- `pkg/routing/router.go` — Memory type classification and routing
- `pkg/provider/unified.go` — UnifiedMemoryProvider orchestrator
- `pkg/provider/adapter.go` — MemoryStore interface adapter
- `pkg/consolidation/consolidation.go` — Sleep-time compute engine
- `pkg/metrics/metrics.go` — Prometheus metrics

### Test Files
- `pkg/types/types_test.go` — Type and interface tests
- `pkg/types/circuit_breaker_test.go` — Circuit breaker tests
- `pkg/config/config_test.go` — Configuration tests
- `pkg/fusion/engine_test.go` — Fusion engine tests
- `pkg/routing/router_test.go` — Router classification tests
- `pkg/provider/unified_test.go` — UnifiedMemoryProvider tests
- `pkg/provider/adapter_test.go` — MemoryStore adapter tests
- `pkg/consolidation/consolidation_test.go` — Consolidation tests

### Infrastructure
- Docker Compose: `docker/docker-compose.yml`
- Services: Letta, Mem0, Cognee, Qdrant, Neo4j, Redis, PostgreSQL

### Standards
- Go 1.24+, standard conventions, testify
- Circuit breakers on all external calls
- Parallel search with errgroup
- Graceful degradation when backends unavailable
- 100% test coverage required
