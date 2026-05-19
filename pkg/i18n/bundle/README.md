# HelixMemory i18n bundle

CONST-046 (No-Hardcoded-Content) resource bundle for HelixMemory.

## Status: skeleton — zero user-facing strings migrated yet

HelixMemory is a programmatic SDK / memory-fusion backend:
- No CLI (no `cmd/` directory)
- No stdout / `fmt.Println` / `fmt.Printf` in production code
- All `fmt.Errorf` surfaces are developer-facing error wraps (operator/dev audience, English-only per CONST-046 §11.4 carve-out for log/operator surfaces)
- All `zap` logger calls are structured operator logs (English-only by design — grep/dedupe/alerting depend on stable English keys)

Per round-210 task scope: a programmatic memory backend with zero user-facing strings is the canonical **no-op infra pattern** — the translator + bundle are wired up so any future user-facing surface (REPL, REST API human-readable messages, future CLI) lands on a CONST-046-clean seam from line one.

## Adding a user-facing string

1. Choose a stable key with the mandatory `helixmemory_` prefix:
   ```
   helixmemory_<area>_<short_identifier>
   ```
   Example: `helixmemory_circuit_breaker_open`.
2. Add the key to every locale file in this directory (`en.yaml` baseline, plus any locales the consumer ships).
3. Use it at the call site via the package-level seam:
   ```go
   return fmt.Errorf("%s", i18n.T("", i18n.BundlePrefix+"circuit_breaker_open"))
   ```
4. Add a regression test verifying the key resolves under at least the `en` locale and falls through to a verbatim key under `NoopTranslator`.

## Why no `en.yaml` yet

Adding an empty `en.yaml` would itself be a CONST-046 trap — every consumer would mistakenly think they should add English strings here directly, defeating the runtime-translation invariant. The file appears the moment the first real user-facing string is added.
