# HelixMemory i18n bundle

CONST-046 (No-Hardcoded-Content) resource bundle for HelixMemory.

## Status: active — round-359 CONST-046 Phase 4 migrated the first 21 keys

`en.yaml` ships the English baseline. Round-359 §11.4 migrated 21 genuinely
user-facing strings off static literals:

- **MCP bridge tool descriptions** (12 keys, `helixmemory_mcp_*`) — surfaced in
  MCP clients via `mcp_bridge.Bridge.ListTools` / `ListToolsLocalized`.
- **Codebase-DNA pattern + convention descriptions** (7 keys,
  `helixmemory_dna_*`) — surfaced in `codebase_dna.Profile` structs.
- **Quality-loop recommended-action descriptions** (2 parametric keys,
  `helixmemory_quality_action_*`) — surfaced in `quality_loop.QualityReport`.

The bundle-backed `BundleTranslator` (`pkg/i18n/bundle.go`) loads these via
`//go:embed` and is the package-default translator, so the SDK surfaces real
English out of the box. Consumers register a locale-aware translator via
`i18n.Set` for additional locales.

The remaining `fmt.Errorf` / `zap` surfaces stay English-only by design:
- All `fmt.Errorf` surfaces are developer-facing error wraps (operator/dev audience, English-only per CONST-046 §11.4 carve-out for log/operator surfaces)
- All `zap` logger calls are structured operator logs (English-only by design — grep/dedupe/alerting depend on stable English keys)
- Prometheus metric `Help:` strings are operator-dashboard metadata, English-only by design.

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
