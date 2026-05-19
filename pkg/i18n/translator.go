// Package i18n provides the user-facing-string translation seam for HelixMemory.
//
// Per root constitution §11.4 (CONST-046 — No-Hardcoded-Content Mandate, Phase 4
// round 210 kick-off), every user-facing string surfaced by an owned-by-us
// submodule MUST be either (a) generated dynamically by an LLM at runtime,
// (b) loaded from an i18n resource bundle with locale-aware overrides, or
// (c) composed programmatically from verifier metadata / configuration data.
// Static literal strings exceeding a length threshold are CONST-046 violations.
//
// HelixMemory is a programmatic SDK / memory-fusion backend with no CLI,
// no stdout/Printf surface, and no end-user prompts: every observable string
// today is either a developer-facing error wrap (`fmt.Errorf("graphiti: ...")`)
// or a structured zap log field (developer/operator audience). Per root
// CLAUDE.md §10.5/§5 and CONST-046, those audiences are explicitly out-of-scope
// for runtime translation — they are read by engineers, ops, and CI logs in
// the canonical English form for grep/dedupe/alerting.
//
// Nonetheless we ship this infra now so:
//   1. CONST-051(B) decoupling stays clean — no consumer (HelixCode et al.)
//      injects its own translator implementation by reaching into the
//      submodule's internals; instead it provides one via Set.
//   2. Any future user-facing surface added to HelixMemory (e.g. a future
//      CLI for `helixmemory dump`, a REST API surfacing human messages, or
//      an interactive REPL) lands on a ready translator seam from line one.
//   3. The audit gate already locks the codebase in CONST-046-clean state
//      so a regression (someone hardcodes a new printable banner) trips the
//      gate, not a post-mortem.
//
// Design follows the canonical 3-layer pattern established across the
// HelixCode programme (round 92 onward): Translator interface + NoopTranslator
// fallback + bundle{prefix=helixmemory_} resource files, with a package-level
// seam (Default / Set / T) so callers do not have to thread context.
package i18n

import (
	"strings"
	"sync"
)

// Translator is the contract every i18n backend MUST satisfy.
//
// Implementations: NoopTranslator (built-in, returns the key verbatim — used
// by tests, by SDK consumers that do not enable translation, and as the
// fallback before any consumer-side translator is registered).
//
// Consumer-supplied implementations (e.g. one fronted by go-i18n, by a
// project-wide YAML bundle, or by an LLM call) MUST honour the contract:
//   - Translate is goroutine-safe.
//   - locale follows BCP-47 (e.g. "en", "en-US", "sr-Cyrl-RS", "ja-JP").
//     Empty locale means "translator's default".
//   - args follow Go's fmt verb semantics; the bundle is the source of the
//     format string, NOT the caller (CONST-046: callers MUST NOT pass
//     pre-formatted English).
//   - On unknown key the Translator MUST return the key verbatim, NOT panic
//     and NOT return empty string. (This keeps NoopTranslator's behaviour
//     identical to a misconfigured real translator — fail-loud-not-silent.)
type Translator interface {
	Translate(locale string, key string, args ...interface{}) string
}

// NoopTranslator is the package-default translator. It returns the key verbatim,
// preserving %v / %d / %s verbs unsubstituted because there is no resource
// bundle to drive the substitution. This is the correct behaviour for the
// programmatic backend audience: the key is a stable identifier (e.g.
// helixmemory_circuit_breaker_open) that ops dashboards / log aggregators
// can pattern-match without locale concern.
//
// CONST-046 compliance: NoopTranslator deliberately does NOT contain any
// hardcoded user-facing English — only the key passes through.
type NoopTranslator struct{}

// Translate satisfies Translator.
func (NoopTranslator) Translate(locale string, key string, args ...interface{}) string {
	_ = locale
	_ = args
	// Strip any leading "helixmemory_" namespace prefix when echoing the key
	// so logs are readable when no real translator is wired up. The prefix
	// stays in the resource bundle for cross-submodule uniqueness.
	return strings.TrimPrefix(key, BundlePrefix)
}

// BundlePrefix is the namespace prefix every HelixMemory i18n key MUST carry.
// Cross-submodule uniqueness is required by the programme's bundle-merger
// (a future tool that unifies all owned-submodule bundles into a single
// translator backend); prefix collisions are a CONST-046 audit failure.
const BundlePrefix = "helixmemory_"

var (
	mu     sync.RWMutex
	active Translator = NoopTranslator{}
)

// Default returns the currently-registered translator (NoopTranslator by default).
// Goroutine-safe.
func Default() Translator {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

// Set registers a new translator and returns the previous one. Pass nil to
// reset to NoopTranslator. Goroutine-safe.
//
// CONST-051(B) reminder: this is the ONLY supported way for a consumer to
// inject its translator — HelixMemory MUST NOT reach back into the parent
// project to look one up. The dependency direction is consumer → submodule,
// never the reverse.
func Set(t Translator) Translator {
	mu.Lock()
	defer mu.Unlock()
	prev := active
	if t == nil {
		active = NoopTranslator{}
	} else {
		active = t
	}
	return prev
}

// T is the package-level convenience seam most call sites should use.
// Equivalent to Default().Translate(locale, key, args...). Empty locale =
// translator's default locale.
//
// Example (future user-facing surface):
//
//	return fmt.Errorf("%s", i18n.T("", i18n.BundlePrefix+"circuit_breaker_open"))
//
// Versus today's developer-facing error which stays untranslated:
//
//	return fmt.Errorf("graphiti: circuit breaker open")  // operator audience, English-only by design
func T(locale string, key string, args ...interface{}) string {
	return Default().Translate(locale, key, args...)
}
