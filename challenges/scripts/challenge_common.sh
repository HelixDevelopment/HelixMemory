#!/bin/bash
# challenge_common.sh — Lightweight challenge framework for HelixMemory module
# Provides color logging, test counting, run_test helper, and summary.

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Project root detection: go up from script dir to HelixMemory root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Timestamp for results
CHALLENGE_START_TIME=$(date +%s)

# --- Logging functions ---

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_header() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
}

# --- Test execution ---

# run_test <test_name> <command...>
# Runs a command, increments counters, logs pass/fail.
run_test() {
    local test_name="$1"
    shift
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    local output
    output=$("$@" 2>&1)
    local exit_code=$?

    if [ $exit_code -eq 0 ]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "$test_name"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "$test_name"
        if [ -n "$output" ]; then
            echo "    Output: $(echo "$output" | head -5)"
        fi
        return 1
    fi
}

# run_test_grep <test_name> <pattern> <file>
# Checks that a grep pattern matches in a file.
run_test_grep() {
    local test_name="$1"
    local pattern="$2"
    local file="$3"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    if grep -q "$pattern" "$file" 2>/dev/null; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "$test_name"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "$test_name"
        echo "    Pattern '$pattern' not found in $file"
        return 1
    fi
}

# run_test_grep_r <test_name> <pattern> <directory>
# Checks that a grep pattern matches recursively in a directory.
run_test_grep_r() {
    local test_name="$1"
    local pattern="$2"
    local directory="$3"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    if grep -rq "$pattern" "$directory" 2>/dev/null; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "$test_name"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        log_error "$test_name"
        echo "    Pattern '$pattern' not found in $directory"
        return 1
    fi
}

# --- Summary ---

# print_summary <challenge_name>
# Prints results and exits with appropriate code (0 = all pass, 1 = failures).
print_summary() {
    local challenge_name="${1:-Challenge}"
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - CHALLENGE_START_TIME))

    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}  $challenge_name - Summary${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
    echo -e "  Total:  ${TESTS_TOTAL}"
    echo -e "  Passed: ${GREEN}${TESTS_PASSED}${NC}"
    echo -e "  Failed: ${RED}${TESTS_FAILED}${NC}"
    echo -e "  Duration: ${duration}s"
    echo ""

    if [ "$TESTS_FAILED" -eq 0 ]; then
        echo -e "${GREEN}ALL ${TESTS_PASSED} TESTS PASSED${NC}"
        exit 0
    else
        echo -e "${RED}${TESTS_FAILED} TEST(S) FAILED${NC}"
        exit 1
    fi
}
