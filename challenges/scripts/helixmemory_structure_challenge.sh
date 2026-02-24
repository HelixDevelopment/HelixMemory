#!/bin/bash
# helixmemory_structure_challenge.sh — Validates HelixMemory module structure
# Checks required files, packages, backend clients, power features, test files,
# and module name correctness.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/challenge_common.sh"

log_header "HelixMemory Structure Challenge"
log_info "Project root: $PROJECT_ROOT"

# ============================================================
# Section 1: Required root files
# ============================================================
log_header "Section 1: Required Root Files"

run_test "go.mod exists" test -f "$PROJECT_ROOT/go.mod"
run_test "go.sum exists" test -f "$PROJECT_ROOT/go.sum"
run_test "CLAUDE.md exists" test -f "$PROJECT_ROOT/CLAUDE.md"
run_test "AGENTS.md exists" test -f "$PROJECT_ROOT/AGENTS.md"
run_test "README.md exists" test -f "$PROJECT_ROOT/README.md"
run_test "docker-compose.yml exists" test -f "$PROJECT_ROOT/docker/docker-compose.yml"

# ============================================================
# Section 2: Required packages
# ============================================================
log_header "Section 2: Required Packages"

REQUIRED_PACKAGES=(
    "pkg/types"
    "pkg/config"
    "pkg/fusion"
    "pkg/routing"
    "pkg/provider"
    "pkg/consolidation"
    "pkg/metrics"
)

for pkg in "${REQUIRED_PACKAGES[@]}"; do
    run_test "Package $pkg exists" test -d "$PROJECT_ROOT/$pkg"
done

# Verify each package has at least one .go file
for pkg in "${REQUIRED_PACKAGES[@]}"; do
    pkg_name=$(basename "$pkg")
    run_test "Package $pkg has Go source files" bash -c "ls '$PROJECT_ROOT/$pkg'/*.go >/dev/null 2>&1"
done

# ============================================================
# Section 3: Backend clients
# ============================================================
log_header "Section 3: Backend Clients (4 required)"

REQUIRED_CLIENTS=(
    "mem0"
    "cognee"
    "letta"
    "graphiti"
)

for client in "${REQUIRED_CLIENTS[@]}"; do
    run_test "Client $client directory exists" test -d "$PROJECT_ROOT/pkg/clients/$client"
    run_test "Client $client has client.go" test -f "$PROJECT_ROOT/pkg/clients/$client/client.go"
    run_test "Client $client has client_test.go" test -f "$PROJECT_ROOT/pkg/clients/$client/client_test.go"
done

# ============================================================
# Section 4: Power features (12 required)
# ============================================================
log_header "Section 4: Power Features (12 required)"

REQUIRED_FEATURES=(
    "codebase_dna"
    "code_gen"
    "confidence"
    "context_window"
    "cross_project"
    "debate_memory"
    "mcp_bridge"
    "mesh"
    "procedural"
    "quality_loop"
    "snapshots"
    "temporal"
)

for feature in "${REQUIRED_FEATURES[@]}"; do
    run_test "Feature $feature directory exists" test -d "$PROJECT_ROOT/pkg/features/$feature"
done

# Verify each feature has Go source and test files
for feature in "${REQUIRED_FEATURES[@]}"; do
    run_test "Feature $feature has Go source" bash -c "ls '$PROJECT_ROOT/pkg/features/$feature'/*.go 2>/dev/null | grep -v '_test.go' | head -1 | grep -q '.'"
    run_test "Feature $feature has test file" bash -c "ls '$PROJECT_ROOT/pkg/features/$feature'/*_test.go >/dev/null 2>&1"
done

# Count features to confirm exactly 12
feature_count=$(find "$PROJECT_ROOT/pkg/features" -mindepth 1 -maxdepth 1 -type d | wc -l)
run_test "Exactly 12 power features present (found: $feature_count)" test "$feature_count" -eq 12

# ============================================================
# Section 5: Test files
# ============================================================
log_header "Section 5: Test Files"

# Package-level test files
REQUIRED_TEST_FILES=(
    "pkg/types/types_test.go"
    "pkg/types/circuit_breaker_test.go"
    "pkg/config/config_test.go"
    "pkg/fusion/engine_test.go"
    "pkg/routing/router_test.go"
    "pkg/provider/unified_test.go"
    "pkg/provider/adapter_test.go"
    "pkg/consolidation/consolidation_test.go"
    "pkg/metrics/metrics_test.go"
)

for tf in "${REQUIRED_TEST_FILES[@]}"; do
    run_test "Test file $tf exists" test -f "$PROJECT_ROOT/$tf"
done

# Test tier directories
REQUIRED_TEST_DIRS=(
    "tests/integration"
    "tests/e2e"
    "tests/stress"
    "tests/benchmark"
    "tests/security"
)

for td in "${REQUIRED_TEST_DIRS[@]}"; do
    run_test "Test directory $td exists" test -d "$PROJECT_ROOT/$td"
done

# ============================================================
# Section 6: Module name validation
# ============================================================
log_header "Section 6: Module Name"

run_test_grep "go.mod declares module digital.vasic.helixmemory" \
    "^module digital.vasic.helixmemory$" "$PROJECT_ROOT/go.mod"

run_test_grep "go.mod requires digital.vasic.memory" \
    "digital.vasic.memory" "$PROJECT_ROOT/go.mod"

run_test_grep "go.mod specifies Go 1.24+" \
    "^go 1.2[4-9]" "$PROJECT_ROOT/go.mod"

# ============================================================
# Section 7: Documentation completeness
# ============================================================
log_header "Section 7: Documentation"

run_test "docs/ directory exists" test -d "$PROJECT_ROOT/docs"
run_test "README.md is non-empty" test -s "$PROJECT_ROOT/README.md"
run_test "CLAUDE.md is non-empty" test -s "$PROJECT_ROOT/CLAUDE.md"
run_test "AGENTS.md is non-empty" test -s "$PROJECT_ROOT/AGENTS.md"

# ============================================================
# Summary
# ============================================================
print_summary "HelixMemory Structure Challenge"
