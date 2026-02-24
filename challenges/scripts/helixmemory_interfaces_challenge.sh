#!/bin/bash
# helixmemory_interfaces_challenge.sh — Validates HelixMemory interface contracts
# Checks that all required interfaces are defined with correct methods, that
# implementations follow patterns (circuit breaker, parallel search, fusion stages),
# and that configuration reads all HELIX_MEMORY_* env vars.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/challenge_common.sh"

log_header "HelixMemory Interfaces Challenge"
log_info "Project root: $PROJECT_ROOT"

PKG="$PROJECT_ROOT/pkg"
TYPES="$PKG/types/types.go"
PROVIDER_UNIFIED="$PKG/provider/unified.go"
PROVIDER_ADAPTER="$PKG/provider/adapter.go"
FUSION_ENGINE="$PKG/fusion/engine.go"
CONFIG_FILE="$PKG/config/config.go"

# ============================================================
# Section 1: MemoryProvider interface
# ============================================================
log_header "Section 1: MemoryProvider Interface"

run_test_grep "MemoryProvider interface defined" \
    "type MemoryProvider interface" "$TYPES"

run_test_grep "MemoryProvider has Name() method" \
    "Name() MemorySource" "$TYPES"

run_test_grep "MemoryProvider has Add() method" \
    "Add(ctx context.Context" "$TYPES"

run_test_grep "MemoryProvider has Search() method" \
    "Search(ctx context.Context" "$TYPES"

run_test_grep "MemoryProvider has Get() method" \
    "Get(ctx context.Context" "$TYPES"

run_test_grep "MemoryProvider has Update() method" \
    "Update(ctx context.Context" "$TYPES"

run_test_grep "MemoryProvider has Delete() method" \
    "Delete(ctx context.Context" "$TYPES"

run_test_grep "MemoryProvider has Health() method" \
    "Health(ctx context.Context" "$TYPES"

# ============================================================
# Section 2: CoreMemoryProvider interface
# ============================================================
log_header "Section 2: CoreMemoryProvider Interface"

run_test_grep "CoreMemoryProvider interface defined" \
    "type CoreMemoryProvider interface" "$TYPES"

run_test_grep "CoreMemoryProvider embeds MemoryProvider" \
    "MemoryProvider" "$TYPES"

run_test_grep "CoreMemoryProvider has GetCoreMemory()" \
    "GetCoreMemory(ctx context.Context" "$TYPES"

run_test_grep "CoreMemoryProvider has UpdateCoreMemory()" \
    "UpdateCoreMemory(ctx context.Context" "$TYPES"

# ============================================================
# Section 3: TemporalProvider interface
# ============================================================
log_header "Section 3: TemporalProvider Interface"

run_test_grep "TemporalProvider interface defined" \
    "type TemporalProvider interface" "$TYPES"

run_test_grep "TemporalProvider has SearchTemporal()" \
    "SearchTemporal(ctx context.Context" "$TYPES"

run_test_grep "TemporalProvider has GetTimeline()" \
    "GetTimeline(ctx context.Context" "$TYPES"

# ============================================================
# Section 4: ConsolidationProvider interface
# ============================================================
log_header "Section 4: ConsolidationProvider Interface"

run_test_grep "ConsolidationProvider interface defined" \
    "type ConsolidationProvider interface" "$TYPES"

run_test_grep "ConsolidationProvider has TriggerConsolidation()" \
    "TriggerConsolidation(ctx context.Context" "$TYPES"

run_test_grep "ConsolidationProvider has GetConsolidationStatus()" \
    "GetConsolidationStatus(ctx context.Context" "$TYPES"

# ============================================================
# Section 5: MemoryStoreAdapter implements MemoryStore
# ============================================================
log_header "Section 5: MemoryStoreAdapter"

run_test_grep "MemoryStoreAdapter struct defined" \
    "type MemoryStoreAdapter struct" "$PROVIDER_ADAPTER"

run_test_grep "MemoryStoreAdapter has Add method" \
    "func (a \*MemoryStoreAdapter) Add(" "$PROVIDER_ADAPTER"

run_test_grep "MemoryStoreAdapter has Search method" \
    "func (a \*MemoryStoreAdapter) Search(" "$PROVIDER_ADAPTER"

run_test_grep "MemoryStoreAdapter has Get method" \
    "func (a \*MemoryStoreAdapter) Get(" "$PROVIDER_ADAPTER"

run_test_grep "MemoryStoreAdapter has Update method" \
    "func (a \*MemoryStoreAdapter) Update(" "$PROVIDER_ADAPTER"

run_test_grep "MemoryStoreAdapter has Delete method" \
    "func (a \*MemoryStoreAdapter) Delete(" "$PROVIDER_ADAPTER"

run_test_grep "MemoryStoreAdapter has List method" \
    "func (a \*MemoryStoreAdapter) List(" "$PROVIDER_ADAPTER"

# ============================================================
# Section 6: UnifiedMemoryProvider has parallel search
# ============================================================
log_header "Section 6: UnifiedMemoryProvider Parallel Search"

run_test_grep "UnifiedMemoryProvider struct defined" \
    "type UnifiedMemoryProvider struct" "$PROVIDER_UNIFIED"

run_test_grep "UnifiedMemoryProvider uses errgroup for parallelism" \
    "errgroup" "$PROVIDER_UNIFIED"

run_test_grep "UnifiedMemoryProvider imports errgroup" \
    "golang.org/x/sync/errgroup" "$PROVIDER_UNIFIED"

run_test_grep "UnifiedMemoryProvider has Search method" \
    "func (u \*UnifiedMemoryProvider) Search(" "$PROVIDER_UNIFIED"

run_test_grep "UnifiedMemoryProvider has Add method" \
    "func (u \*UnifiedMemoryProvider) Add(" "$PROVIDER_UNIFIED"

# ============================================================
# Section 7: Fusion engine has 3 stages
# ============================================================
log_header "Section 7: Fusion Engine (3 Stages)"

run_test_grep "Fusion Engine struct defined" \
    "type Engine struct" "$FUSION_ENGINE"

run_test_grep "Fusion Engine has Fuse method" \
    "func (e \*Engine) Fuse(" "$FUSION_ENGINE"

run_test_grep "Stage 1: collect (Collection & Normalization)" \
    "collect" "$FUSION_ENGINE"

run_test_grep "Stage 2: deduplicate (Deduplication)" \
    "deduplicate" "$FUSION_ENGINE"

run_test_grep "Stage 3: rerank (Cross-Source Re-Ranking)" \
    "rerank" "$FUSION_ENGINE"

run_test_grep "Dedup threshold field present" \
    "dedupThreshold" "$FUSION_ENGINE"

# ============================================================
# Section 8: Circuit breaker in all 4 clients
# ============================================================
log_header "Section 8: Circuit Breaker Pattern"

CLIENTS=(mem0 cognee letta graphiti)

for client in "${CLIENTS[@]}"; do
    client_file="$PKG/clients/$client/client.go"
    run_test_grep "Client $client has CircuitBreaker field" \
        "breaker.*CircuitBreaker" "$client_file"
    run_test_grep "Client $client creates CircuitBreaker" \
        "NewCircuitBreaker" "$client_file"
done

# Verify CircuitBreaker type exists
run_test_grep "CircuitBreaker type defined" \
    "type CircuitBreaker struct" "$PKG/types/circuit_breaker.go"

run_test_grep "CircuitBreaker has state management" \
    "CircuitState" "$PKG/types/circuit_breaker.go"

# ============================================================
# Section 9: Configuration reads HELIX_MEMORY_* env vars
# ============================================================
log_header "Section 9: Configuration Env Vars"

REQUIRED_ENV_VARS=(
    "HELIX_MEMORY_LETTA_ENDPOINT"
    "HELIX_MEMORY_MEM0_ENDPOINT"
    "HELIX_MEMORY_COGNEE_ENDPOINT"
    "HELIX_MEMORY_GRAPHITI_ENDPOINT"
    "HELIX_MEMORY_QDRANT_ENDPOINT"
    "HELIX_MEMORY_NEO4J_ENDPOINT"
    "HELIX_MEMORY_NEO4J_USER"
    "HELIX_MEMORY_NEO4J_PASSWORD"
    "HELIX_MEMORY_REDIS_ENDPOINT"
    "HELIX_MEMORY_REDIS_PASSWORD"
    "HELIX_MEMORY_FUSION_DEDUP_THRESHOLD"
    "HELIX_MEMORY_CONSOLIDATION_ENABLED"
    "HELIX_MEMORY_CONSOLIDATION_INTERVAL"
    "HELIX_MEMORY_DEFAULT_TOP_K"
    "HELIX_MEMORY_REQUEST_TIMEOUT"
    "HELIX_MEMORY_ENABLE_METRICS"
    "HELIX_MEMORY_EMBEDDING_MODEL"
    "HELIX_MEMORY_EMBEDDING_ENDPOINT"
    "HELIX_MEMORY_EMBEDDING_DIMENSION"
    "HELIX_MEMORY_CIRCUIT_BREAKER_THRESHOLD"
    "HELIX_MEMORY_CIRCUIT_BREAKER_TIMEOUT"
)

for env_var in "${REQUIRED_ENV_VARS[@]}"; do
    run_test_grep "Config reads $env_var" \
        "$env_var" "$CONFIG_FILE"
done

# Verify FromEnv function exists
run_test_grep "FromEnv() function defined" \
    "func FromEnv()" "$CONFIG_FILE"

# Verify DefaultConfig function exists
run_test_grep "DefaultConfig() function defined" \
    "func DefaultConfig()" "$CONFIG_FILE"

# ============================================================
# Summary
# ============================================================
print_summary "HelixMemory Interfaces Challenge"
