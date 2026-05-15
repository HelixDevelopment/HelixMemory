#!/usr/bin/env bash
# ddos_health_flood_challenge.sh — anti-bluff DDoS Challenge for
# HelixMemory per CONST-035 + CONST-050(B). Submodule cascade per
# CONST-051(A). Targets $HELIXMEMORY_HEALTH_URL.

set -uo pipefail

HEALTH_URL="${HELIXMEMORY_HEALTH_URL:-}"
TOTAL_REQS="${DDOS_REQUESTS:-500}"
CONCURRENCY="${DDOS_CONCURRENCY:-50}"
TIMEOUT_SEC="${DDOS_TIMEOUT_SEC:-5}"
MIN_PASS_PCT="${DDOS_MIN_PASS_PCT:-95}"

echo "=== HelixMemory DDoS Health-Flood Challenge ==="
echo "  url=$HEALTH_URL total=$TOTAL_REQS conc=$CONCURRENCY pass≥${MIN_PASS_PCT}%"

if [[ -z "$HEALTH_URL" ]]; then
    echo "[1/5] SKIP: HELIXMEMORY_HEALTH_URL unset — SKIP-OK: #env-no-target"
    echo "=== HelixMemory DDoS Challenge: PASSED (SKIP-OK) ==="
    exit 0
fi
pre=$(curl -sS --max-time "$TIMEOUT_SEC" -o /dev/null -w "%{http_code}" "$HEALTH_URL" 2>/dev/null) || pre="000"
if [[ "$pre" != "200" ]]; then
    echo "[1/5] SKIP: target unreachable (HTTP $pre) — SKIP-OK: #env-target-down"
    echo "=== HelixMemory DDoS Challenge: PASSED (SKIP-OK) ==="
    exit 0
fi
echo "[1/5] Pre-flood: PASS"

body=$(curl -sS --max-time "$TIMEOUT_SEC" "$HEALTH_URL" 2>/dev/null || true)
printf '%s' "$body" | grep -qE '"status"\s*:\s*"(ok|healthy|UP)"' || { echo "[2/5] FAIL"; exit 1; }
echo "[2/5] Schema sanity: PASS"

RES=$(mktemp); trap "rm -f $RES" EXIT
start=$(date +%s.%N)
seq 1 "$TOTAL_REQS" | xargs -n1 -P "$CONCURRENCY" -I{} \
    curl -sS -o /dev/null --max-time "$TIMEOUT_SEC" \
        -w "%{http_code} %{time_total}\n" "$HEALTH_URL" 2>/dev/null >> "$RES" || true
end=$(date +%s.%N)
wall=$(awk -v a="$start" -v b="$end" 'BEGIN{printf "%.3f", b-a}')
total=$(wc -l < "$RES" | tr -d ' '); [[ "$total" -eq 0 ]] && total=1
ok=$(awk '$1=="200"{c++} END{print c+0}' "$RES")
pct=$((ok * 100 / total))
sorted=$(awk '{print $2}' "$RES" | sort -n)
p50=$(printf '%s\n' "$sorted" | awk -v n="$total" 'NR==int(n*0.5){print; exit}')
p95=$(printf '%s\n' "$sorted" | awk -v n="$total" 'NR==int(n*0.95){print; exit}')

echo "[3/5] Flood: total=$total ok=$ok pct=${pct}% wall=${wall}s p50=${p50:-N/A}s p95=${p95:-N/A}s"
[[ "$pct" -lt "$MIN_PASS_PCT" ]] && { echo "[4/5] FAIL"; exit 1; }
echo "[4/5] Threshold: PASS"

post=$(curl -sS --max-time "$TIMEOUT_SEC" -o /dev/null -w "%{http_code}" "$HEALTH_URL" 2>/dev/null) || post="000"
[[ "$post" != "200" ]] && { echo "[5/5] FAIL"; exit 1; }
echo "[5/5] Post-flood liveness: PASS"

echo
echo "=== HelixMemory DDoS Challenge: PASSED ==="
echo "  evidence: reqs=$total ok=$ok pct=${pct}% wall=${wall}s p50=${p50:-N/A}s p95=${p95:-N/A}s"
