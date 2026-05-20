package i18n

import (
	"strings"
	"testing"
)

// roundParametricKeys are the format-string keys migrated in round-359 — they
// take fmt args and so cannot be asserted by a plain equality check.
var roundParametricKeys = map[string]struct{}{
	"helixmemory_quality_action_prune_stale":             {},
	"helixmemory_quality_action_validate_low_confidence": {},
}

// round359MigratedKeys is the full set of user-facing keys migrated off
// hardcoded literals in round-359. Every one MUST resolve to a non-empty,
// non-verbatim English string under the embedded bundle — that is the
// CONST-046 proof that the literal really moved into the bundle.
var round359MigratedKeys = []string{
	"helixmemory_mcp_tool_search_desc",
	"helixmemory_mcp_tool_add_desc",
	"helixmemory_mcp_tool_health_desc",
	"helixmemory_mcp_tool_get_desc",
	"helixmemory_mcp_tool_delete_desc",
	"helixmemory_mcp_param_query_desc",
	"helixmemory_mcp_param_top_k_desc",
	"helixmemory_mcp_param_user_filter_desc",
	"helixmemory_mcp_param_content_desc",
	"helixmemory_mcp_param_mem_type_desc",
	"helixmemory_mcp_param_user_id_desc",
	"helixmemory_mcp_param_memory_id_desc",
	"helixmemory_dna_pattern_interface_abstraction",
	"helixmemory_dna_pattern_table_driven_tests",
	"helixmemory_dna_pattern_context_propagation",
	"helixmemory_dna_pattern_mutex_concurrency",
	"helixmemory_dna_pattern_error_wrapping",
	"helixmemory_dna_pattern_abstract_base_classes",
	"helixmemory_dna_convention_line_length",
	"helixmemory_quality_action_prune_stale",
	"helixmemory_quality_action_validate_low_confidence",
}

// TestNewBundleTranslator_LoadsEmbeddedEn confirms the embedded en.yaml loads
// and the default-locale guard accepts it.
func TestNewBundleTranslator_LoadsEmbeddedEn(t *testing.T) {
	bt, err := NewBundleTranslator("en")
	if err != nil {
		t.Fatalf("NewBundleTranslator(en) must succeed with embedded bundle: %v", err)
	}
	if _, ok := bt.locales["en"]; !ok {
		t.Fatalf("BundleTranslator did not load the en locale; have %v", bt.localeNames())
	}
}

// TestNewBundleTranslator_RejectsMissingDefault is the paired mutation for the
// fail-loud default-locale guard: a default locale with no bundle file MUST
// error rather than silently returning a translator that can never fall back.
func TestNewBundleTranslator_RejectsMissingDefault(t *testing.T) {
	_, err := NewBundleTranslator("zz-NoSuchLocale")
	if err == nil {
		t.Fatal("NewBundleTranslator MUST error when the default locale has no bundle file")
	}
}

// TestRound359_AllMigratedKeysResolve is the core CONST-046 proof: every key
// migrated in round-359 resolves to a real English string under the embedded
// bundle — NOT the verbatim prefix-stripped key. If a key were missing from
// en.yaml, T() would fall through to the bare key and this test fires.
func TestRound359_AllMigratedKeysResolve(t *testing.T) {
	resetTranslator(t)
	for _, key := range round359MigratedKeys {
		bare := strings.TrimPrefix(key, BundlePrefix)
		var got string
		if _, parametric := roundParametricKeys[key]; parametric {
			// Parametric keys carry fmt verbs; pass plausible args so the
			// rendered string differs from both the raw format and the key.
			got = T("en", key, 7, "30d")
		} else {
			got = T("en", key)
		}
		if got == "" {
			t.Errorf("key %q resolved to empty string — CONST-046 contract violation", key)
		}
		if got == bare {
			t.Errorf("key %q fell through to the verbatim key %q — not migrated into the bundle", key, bare)
		}
	}
}

// TestPairedMutation_MissingBundleKeyFallsThrough plants the canonical
// regression: a key that is NOT in any bundle file. The contract requires
// fall-through to the verbatim prefix-stripped key (never empty, never panic).
// This is the mutation that proves TestRound359_AllMigratedKeysResolve has
// teeth — if a migrated key were dropped from en.yaml, it would behave
// exactly like this planted-missing key and the resolve test would FAIL.
func TestPairedMutation_MissingBundleKeyFallsThrough(t *testing.T) {
	resetTranslator(t)
	const planted = BundlePrefix + "round359_deliberately_absent_key"
	got := T("en", planted)
	if got != "round359_deliberately_absent_key" {
		t.Fatalf("missing-key fall-through mismatch: got %q want %q",
			got, "round359_deliberately_absent_key")
	}
}

// TestBundleTranslator_LocaleFallbackChain exercises exact → primary-subtag →
// default-locale resolution. "sr-Cyrl-RS" has no bundle, so a known key MUST
// still resolve via the "en" default.
func TestBundleTranslator_LocaleFallbackChain(t *testing.T) {
	bt, err := NewBundleTranslator("en")
	if err != nil {
		t.Fatalf("NewBundleTranslator(en): %v", err)
	}
	got := bt.Translate("sr-Cyrl-RS", "helixmemory_mcp_tool_health_desc")
	if got == "" || got == "mcp_tool_health_desc" {
		t.Fatalf("locale fallback to en failed: got %q", got)
	}
}

// TestBundleTranslator_ParametricRendering confirms fmt args are applied to
// the bundle's format string (the bundle owns the format, the caller the args).
func TestBundleTranslator_ParametricRendering(t *testing.T) {
	bt, err := NewBundleTranslator("en")
	if err != nil {
		t.Fatalf("NewBundleTranslator(en): %v", err)
	}
	got := bt.Translate("en", "helixmemory_quality_action_prune_stale", 12, "30d")
	if !strings.Contains(got, "12") || !strings.Contains(got, "30d") {
		t.Fatalf("parametric rendering did not apply fmt args: got %q", got)
	}
}
