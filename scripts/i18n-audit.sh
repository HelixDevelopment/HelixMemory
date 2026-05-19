#!/usr/bin/env bash
# i18n-audit.sh — CONST-046 No-Hardcoded-Content audit gate for HelixMemory.
#
# Scope: catch new user-facing hardcoded strings that should go through the
# pkg/i18n translator seam instead. HelixMemory is currently a programmatic
# SDK with zero user-facing surface (no CLI, no stdout, no REST text surface),
# so the audit's job today is to detect a regression: someone adding a
# `fmt.Println` / `fmt.Printf` / interactive REPL prompt without routing it
# through `i18n.T(...)`.
#
# Developer-facing surfaces are out-of-scope by CONST-046 §11.4 carve-out:
#   - fmt.Errorf(...) error wraps  → operator/dev audience, English-only
#   - zap log calls (Info/Warn/Error/...) → ops/CI logs, English-only
#   - log.Println / log.Printf via stdlib for boot diagnostics → operator audience
#
# Exit codes:
#   0 — clean
#   1 — at least one CONST-046 violation found
#
# Paired-mutation harness:
#   --self-test  plant a violation in a throwaway file, confirm we exit 1,
#                clean up, then re-run clean and confirm we exit 0.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PKG_ROOT="$ROOT/pkg"
EXIT=0

scan() {
  local violations
  # The forbidden patterns: fmt.Println / fmt.Printf / fmt.Print / fmt.Fprintln(os.Stdout
  # in production .go files (not _test.go) under pkg/.
  # `i18n.T(` on the same line is the escape hatch — the string IS already
  # routed through the translator.
  violations=$(
    grep -rn -E '\b(fmt\.(Println|Printf|Print|Fprintln|Fprintf))\b' \
      --include='*.go' \
      --exclude='*_test.go' \
      "$PKG_ROOT" 2>/dev/null \
      | grep -v 'i18n\.T(' \
      | grep -v 'os\.Stderr' \
      | grep -v '// CONST-046-OK:' \
      || true
  )
  if [[ -n "$violations" ]]; then
    echo "CONST-046 i18n-audit: hardcoded user-facing print(s) found in production source:" >&2
    echo "$violations" >&2
    echo "" >&2
    echo "Fix: route the string through pkg/i18n.T(\"\", i18n.BundlePrefix+\"<key>\") instead," >&2
    echo "     or annotate the line with the trailing comment '// CONST-046-OK: <justification>'" >&2
    echo "     if it is genuinely a developer/operator surface." >&2
    return 1
  fi
  return 0
}

self_test() {
  local tmp="$PKG_ROOT/i18n/.audit_selftest.go"
  cat >"$tmp" <<'GO'
package i18n

import "fmt"

func planted_violation_for_const046_audit_selftest() {
	fmt.Println("this is a hardcoded user-facing string — must be caught")
}
GO
  local rc=0
  scan >/dev/null 2>&1 || rc=$?
  rm -f "$tmp"
  if [[ $rc -ne 1 ]]; then
    echo "PAIRED-MUTATION FAIL: planted violation was NOT caught (audit exit code $rc, expected 1)" >&2
    return 2
  fi
  # Now confirm clean re-run.
  if ! scan >/dev/null 2>&1; then
    echo "PAIRED-MUTATION FAIL: clean source scan unexpectedly reported violations" >&2
    return 3
  fi
  echo "paired-mutation OK: planted violation caught, clean scan clean"
  return 0
}

case "${1:-}" in
  --self-test)
    self_test
    exit $?
    ;;
  *)
    if scan; then
      echo "CONST-046 i18n-audit: clean (no hardcoded user-facing prints in pkg/)"
      exit 0
    else
      exit 1
    fi
    ;;
esac
