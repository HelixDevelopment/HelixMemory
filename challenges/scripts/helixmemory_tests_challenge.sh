#!/bin/bash
# helixmemory_tests_challenge.sh — Validates HelixMemory test execution
# Checks build, vet, unit tests, integration tests, e2e tests, stress tests,
# benchmarks, minimum test count, and zero failures.
# All runs use GOMAXPROCS=2 and -p 1 per resource management rules.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/challenge_common.sh"

log_header "HelixMemory Tests Challenge"
log_info "Project root: $PROJECT_ROOT"

# Resource limits per project constitution
export GOMAXPROCS=2
NICE_PREFIX="nice -n 19 ionice -c 3"
GO_TEST_FLAGS="-race -count=1 -p 1 -timeout 300s"

cd "$PROJECT_ROOT"

# ============================================================
# Section 1: Build verification
# ============================================================
log_header "Section 1: Build Verification"

run_test "go build ./... succeeds" $NICE_PREFIX go build ./...

# ============================================================
# Section 2: Code quality
# ============================================================
log_header "Section 2: Code Quality"

run_test "go vet ./... passes" $NICE_PREFIX go vet ./...

# ============================================================
# Section 3: Unit tests (pkg/)
# ============================================================
log_header "Section 3: Unit Tests"

UNIT_OUTPUT=$(mktemp)
log_info "Running unit tests with GOMAXPROCS=2 -race -count=1 -p 1 ..."

if $NICE_PREFIX go test $GO_TEST_FLAGS ./pkg/... > "$UNIT_OUTPUT" 2>&1; then
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_success "All pkg/ unit tests pass"
else
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "pkg/ unit tests failed"
    tail -20 "$UNIT_OUTPUT"
fi

# Check for zero failures in output
if grep -q "^FAIL" "$UNIT_OUTPUT" 2>/dev/null; then
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "FAIL lines detected in unit test output"
    grep "^FAIL" "$UNIT_OUTPUT"
else
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_success "No FAIL lines in unit test output"
fi

rm -f "$UNIT_OUTPUT"

# ============================================================
# Section 4: Integration tests
# ============================================================
log_header "Section 4: Integration Tests"

if [ -d "$PROJECT_ROOT/tests/integration" ] && ls "$PROJECT_ROOT/tests/integration"/*_test.go >/dev/null 2>&1; then
    INTEG_OUTPUT=$(mktemp)
    log_info "Running integration tests ..."

    if $NICE_PREFIX go test $GO_TEST_FLAGS ./tests/integration/... > "$INTEG_OUTPUT" 2>&1; then
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "Integration tests pass"
    else
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "Integration tests failed"
        tail -20 "$INTEG_OUTPUT"
    fi
    rm -f "$INTEG_OUTPUT"
else
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "No integration test files found in tests/integration/"
fi

# ============================================================
# Section 5: E2E tests
# ============================================================
log_header "Section 5: E2E Tests"

if [ -d "$PROJECT_ROOT/tests/e2e" ] && ls "$PROJECT_ROOT/tests/e2e"/*_test.go >/dev/null 2>&1; then
    E2E_OUTPUT=$(mktemp)
    log_info "Running e2e tests ..."

    if $NICE_PREFIX go test $GO_TEST_FLAGS ./tests/e2e/... > "$E2E_OUTPUT" 2>&1; then
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "E2E tests pass"
    else
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "E2E tests failed"
        tail -20 "$E2E_OUTPUT"
    fi
    rm -f "$E2E_OUTPUT"
else
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "No e2e test files found in tests/e2e/"
fi

# ============================================================
# Section 6: Stress tests
# ============================================================
log_header "Section 6: Stress Tests"

if [ -d "$PROJECT_ROOT/tests/stress" ] && ls "$PROJECT_ROOT/tests/stress"/*_test.go >/dev/null 2>&1; then
    STRESS_OUTPUT=$(mktemp)
    log_info "Running stress tests ..."

    if $NICE_PREFIX go test $GO_TEST_FLAGS ./tests/stress/... > "$STRESS_OUTPUT" 2>&1; then
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "Stress tests pass"
    else
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "Stress tests failed"
        tail -20 "$STRESS_OUTPUT"
    fi
    rm -f "$STRESS_OUTPUT"
else
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "No stress test files found in tests/stress/"
fi

# ============================================================
# Section 7: Benchmarks
# ============================================================
log_header "Section 7: Benchmarks"

if [ -d "$PROJECT_ROOT/tests/benchmark" ] && ls "$PROJECT_ROOT/tests/benchmark"/*_test.go >/dev/null 2>&1; then
    BENCH_OUTPUT=$(mktemp)
    log_info "Running benchmarks (quick, -benchtime=1x) ..."

    if $NICE_PREFIX go test -bench=. -benchtime=1x -run='^$' -p 1 -timeout 120s ./tests/benchmark/... > "$BENCH_OUTPUT" 2>&1; then
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "Benchmarks compile and run"
    else
        TESTS_TOTAL=$((TESTS_TOTAL + 1))
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "Benchmarks failed"
        tail -20 "$BENCH_OUTPUT"
    fi
    rm -f "$BENCH_OUTPUT"
else
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "No benchmark test files found in tests/benchmark/"
fi

# ============================================================
# Section 8: Minimum test count
# ============================================================
log_header "Section 8: Test Count Validation"

ALL_OUTPUT=$(mktemp)
log_info "Running all tests with -v to count ..."

$NICE_PREFIX go test -count=1 -p 1 -timeout 300s -v ./... > "$ALL_OUTPUT" 2>&1 || true

# Count lines matching "--- PASS:" or "--- FAIL:" to get individual test count
PASS_COUNT=$(grep -c "^--- PASS:" "$ALL_OUTPUT" 2>/dev/null) || PASS_COUNT=0
FAIL_COUNT=$(grep -c "^--- FAIL:" "$ALL_OUTPUT" 2>/dev/null) || FAIL_COUNT=0
TOTAL_INDIVIDUAL=$((PASS_COUNT + FAIL_COUNT))

log_info "Individual test results: $PASS_COUNT passed, $FAIL_COUNT failed, $TOTAL_INDIVIDUAL total"

# Minimum 200 total tests
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [ "$TOTAL_INDIVIDUAL" -ge 200 ]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_success "Minimum 200 tests met (found: $TOTAL_INDIVIDUAL)"
else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "Minimum 200 tests NOT met (found: $TOTAL_INDIVIDUAL, need 200)"
fi

# Zero failures
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [ "$FAIL_COUNT" -eq 0 ]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_success "Zero test failures"
else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_error "$FAIL_COUNT test(s) failed"
    grep "^--- FAIL:" "$ALL_OUTPUT" | head -10
fi

rm -f "$ALL_OUTPUT"

# ============================================================
# Summary
# ============================================================
print_summary "HelixMemory Tests Challenge"
