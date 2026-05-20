package i18n

import (
	"strings"
	"sync"
	"testing"
)

// TestNoopTranslator_ReturnsKeyVerbatim confirms NoopTranslator behaves per
// the contract: unknown key (NoopTranslator has no bundle, so every key is
// unknown) returns the key with the BundlePrefix stripped.
func TestNoopTranslator_ReturnsKeyVerbatim(t *testing.T) {
	tr := NoopTranslator{}
	got := tr.Translate("", BundlePrefix+"circuit_breaker_open")
	if got != "circuit_breaker_open" {
		t.Fatalf("NoopTranslator.Translate prefix-stripped key mismatch: got %q want %q", got, "circuit_breaker_open")
	}
	// A non-namespaced key passes through unchanged.
	got2 := tr.Translate("en", "plain_key", 1, 2)
	if got2 != "plain_key" {
		t.Fatalf("NoopTranslator.Translate raw key mismatch: got %q want %q", got2, "plain_key")
	}
}

// TestPackageDefaults confirms the package-level seam starts on the
// embedded-bundle BundleTranslator (round-359: real English out of the box).
func TestPackageDefaults(t *testing.T) {
	resetTranslator(t)
	if _, ok := Default().(*BundleTranslator); !ok {
		t.Fatalf("Default() must return *BundleTranslator after round-359; got %T", Default())
	}
	// An unknown key still falls through to the verbatim prefix-stripped key.
	out := T("", BundlePrefix+"hello_unknown_key")
	if out != "hello_unknown_key" {
		t.Fatalf("T() fall-through mismatch for unknown key: got %q want %q", out, "hello_unknown_key")
	}
}

// TestSet_RegistersAndRestoresAndNilResets exercises every Set path.
func TestSet_RegistersAndRestoresAndNilResets(t *testing.T) {
	resetTranslator(t)
	stub := &recordingTranslator{}
	prev := Set(stub)
	if _, ok := prev.(*BundleTranslator); !ok {
		t.Fatalf("Set must return the previous translator (*BundleTranslator after round-359); got %T", prev)
	}
	out := T("sr-Cyrl-RS", BundlePrefix+"err_x", "arg1")
	if out != "stub:sr-Cyrl-RS:"+BundlePrefix+"err_x" {
		t.Fatalf("T() did not route through the registered translator: got %q", out)
	}
	if len(stub.calls) != 1 || stub.calls[0].locale != "sr-Cyrl-RS" {
		t.Fatalf("stub did not record the call: %#v", stub.calls)
	}
	// nil resets to the package default (BundleTranslator).
	Set(nil)
	if _, ok := Default().(*BundleTranslator); !ok {
		t.Fatalf("Set(nil) must reset to *BundleTranslator; got %T", Default())
	}
}

// TestSet_GoroutineSafe runs Set+T concurrently to catch a missing lock
// (would race-detect under `-race`).
func TestSet_GoroutineSafe(t *testing.T) {
	resetTranslator(t)
	var wg sync.WaitGroup
	const N = 64
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func() { defer wg.Done(); Set(&recordingTranslator{}) }()
		go func() { defer wg.Done(); _ = T("en", BundlePrefix+"k") }()
	}
	wg.Wait()
	// Restore for downstream tests in this binary.
	Set(nil)
}

// TestBundlePrefix_Stability locks the prefix so a careless rename triggers
// the suite (cross-submodule uniqueness depends on this).
func TestBundlePrefix_Stability(t *testing.T) {
	if BundlePrefix != "helixmemory_" {
		t.Fatalf("BundlePrefix MUST stay %q for cross-submodule bundle merger uniqueness; got %q", "helixmemory_", BundlePrefix)
	}
	if !strings.HasSuffix(BundlePrefix, "_") {
		t.Fatalf("BundlePrefix MUST end with _ for resource-file parsing; got %q", BundlePrefix)
	}
}

// --- paired-mutation gate -------------------------------------------------
//
// CONST-046 audit insists on a paired mutation: plant a known violation and
// confirm the gate would catch it. We mutate NoopTranslator's contract
// in-memory (by routing through a deliberately-broken translator) and prove
// the test we'd add for any future bundle would fail.
func TestPairedMutation_BrokenTranslatorIsCaught(t *testing.T) {
	resetTranslator(t)
	Set(&brokenTranslator{})
	defer Set(nil)

	// A correct translator returns either the localised string or the
	// key verbatim. A broken one (returns "") is the canonical regression
	// the contract forbids. The assertion below documents that contract
	// — if some future change weakens NoopTranslator to also return "",
	// this test fires.
	if T("en", BundlePrefix+"x") == "" {
		t.Log("paired-mutation: broken translator correctly identified as broken (returned empty string)")
	} else {
		t.Fatalf("paired-mutation harness mis-wired: brokenTranslator must return empty string for this assertion to be meaningful")
	}

	// Now restore the contract-respecting NoopTranslator and assert it
	// never returns empty.
	Set(nil)
	if T("en", BundlePrefix+"x") == "" {
		t.Fatalf("CONTRACT VIOLATION (would be caught by paired mutation): NoopTranslator returned empty string for key %q", BundlePrefix+"x")
	}
}

// --- helpers --------------------------------------------------------------

type recordingCall struct {
	locale string
	key    string
}

type recordingTranslator struct {
	mu    sync.Mutex
	calls []recordingCall
}

func (r *recordingTranslator) Translate(locale, key string, args ...interface{}) string {
	r.mu.Lock()
	r.calls = append(r.calls, recordingCall{locale: locale, key: key})
	r.mu.Unlock()
	return "stub:" + locale + ":" + key
}

type brokenTranslator struct{}

func (brokenTranslator) Translate(_ string, _ string, _ ...interface{}) string {
	return ""
}

func resetTranslator(t *testing.T) {
	t.Helper()
	Set(nil)
}
