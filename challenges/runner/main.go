// Command runner is the HelixMemory round-274 Challenge runner.
//
// It exercises the real routing.Router, the real fusion.FusionEngine.Fuse
// stage, the real i18n.NoopTranslator, and the real
// types.CircuitBreaker state machine across five locale fixtures. Every
// PASS line is backed by a runtime invariant — never a metadata-only
// check (CONST-035 / Article XI §11.9).
//
// Anti-bluff invariants enforced:
//
//  1. routing.Router.ClassifyMemoryType(prompt) returns the documented
//     types.MemoryType for each fixture, AND the prompt actually
//     contains the fixture's expect_keyword (proves the lexicon really
//     fires; not a tautology).
//
//  2. fusion.FusionEngine.Fuse over two SearchResults carrying
//     identical-content entries deduplicates to expect_fused_count_same;
//     Fuse over two SearchResults carrying different-content entries
//     yields expect_fused_count_diff. Proves the dedup stage actually
//     runs, not just len(a)+len(b).
//
//  3. i18n.NoopTranslator.Translate returns the key with the
//     "helixmemory_" namespace prefix stripped — the documented
//     anti-bluff contract (translator.go).
//
//  4. types.CircuitBreaker state machine: closed → open after N failures;
//     Allow() returns false in open state.
//
//  5. i18n.BundlePrefix is exactly "helixmemory_" (cross-submodule
//     uniqueness invariant for the future bundle-merger).
//
// Mutation hook: when env HELIXMEMORY_MUTATE_RUNNER=1 is set, the
// runner inverts invariant 3 (treats a successful key-roundtrip as
// FAIL instead of PASS). Paired Challenge wraps this to assert the
// runner exits non-zero under mutation, guaranteeing the runner
// actually checks what it claims (CONST-050(A) paired mutation, §1.1).
//
// Verbatim 2026-05-19 operator mandate (preserved per
// CONST-049 §11.4.17):
//
//	"all existing tests and Challenges do work in anti-bluff
//	manner - they MUST confirm that all tested codebase really
//	works as expected! We had been in position that all tests
//	do execute with success and all Challenges as well, but
//	in reality the most of the features does not work and
//	can't be used! This MUST NOT be the case and execution
//	of tests and Challenges MUST guarantee the quality, the
//	completition and full usability by end users of the
//	product!"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/fusion"
	"digital.vasic.helixmemory/pkg/i18n"
	"digital.vasic.helixmemory/pkg/routing"
	"digital.vasic.helixmemory/pkg/types"
)

// fixture is the 8-field projection of the per-locale JSON record
// shipped at tests/fixtures/helixmemory/payloads.json.
type fixture struct {
	Locale               string `json:"locale"`
	Prompt               string `json:"prompt"`
	ExpectType           string `json:"expect_type"`
	ExpectKeyword        string `json:"expect_keyword"`
	ExpectFusedCountSame int    `json:"expect_fused_count_same"`
	ExpectFusedCountDiff int    `json:"expect_fused_count_diff"`
	ExpectKey            string `json:"expect_key"`
	ExpectTranslated     string `json:"expect_translated"`
}

func main() {
	if code := run(os.Stdout); code != 0 {
		os.Exit(code)
	}
}

func run(out io.Writer) int {
	fmt.Fprintln(out, "=== HelixMemory Challenge Runner (round-274) ===")

	fixPath := os.Getenv("HELIXMEMORY_FIXTURES")
	if fixPath == "" {
		// Default: tests/fixtures/helixmemory/payloads.json relative
		// to the module root. When invoked via `go run
		// ./challenges/runner` this resolves correctly.
		fixPath = filepath.Join(
			"tests", "fixtures", "helixmemory", "payloads.json")
	}
	fixtures, err := loadFixtures(fixPath)
	if err != nil {
		fmt.Fprintf(out, "FAIL: load fixtures from %s: %v\n",
			fixPath, err)
		return 1
	}
	if len(fixtures) < 5 {
		fmt.Fprintf(out, "FAIL: expected >=5 fixtures, got %d\n",
			len(fixtures))
		return 1
	}
	fmt.Fprintf(out, "[setup] loaded %d locale fixtures from %s\n",
		len(fixtures), fixPath)

	mutate := os.Getenv("HELIXMEMORY_MUTATE_RUNNER") == "1"
	if mutate {
		fmt.Fprintln(out, "[setup] MUTATION MODE: runner will treat"+
			" successful key-roundtrip as FAIL")
	}

	router := routing.NewRouter()
	tr := i18n.NoopTranslator{}
	cfg := config.DefaultConfig()
	eng := fusion.NewEngine(cfg)

	pass, fail := 0, 0
	step := func(name string, ok bool, detail string) {
		if ok {
			pass++
			fmt.Fprintf(out, "  PASS  %-52s  %s\n", name, detail)
			return
		}
		fail++
		fmt.Fprintf(out, "  FAIL  %-52s  %s\n", name, detail)
	}

	// Invariant 5: BundlePrefix sanity (cheap, runs once).
	step("i18n.BundlePrefix.is_helixmemory_",
		i18n.BundlePrefix == "helixmemory_",
		fmt.Sprintf("got=%q", i18n.BundlePrefix))

	for _, f := range fixtures {
		// Invariant 1a: lexicon must actually contain the
		// keyword that triggered the classification. Defends
		// against a fixture that claims a keyword but doesn't
		// include it (the test would otherwise be a tautology).
		hasKeyword := strings.Contains(
			strings.ToLower(f.Prompt),
			strings.ToLower(f.ExpectKeyword))
		step("router.fixture_contains_keyword."+f.Locale,
			hasKeyword,
			fmt.Sprintf("prompt=%q keyword=%q",
				f.Prompt, f.ExpectKeyword))

		// Invariant 1b: classifier returns documented type.
		got := router.ClassifyMemoryType(f.Prompt)
		step("router.ClassifyMemoryType."+f.Locale,
			string(got) == f.ExpectType,
			fmt.Sprintf("want=%s got=%s prompt=%q",
				f.ExpectType, got, f.Prompt))

		// Invariant 2a: Fuse over identical entries dedups.
		entry := &types.MemoryEntry{
			ID:         "fixture-" + f.Locale + "-a",
			Content:    f.Prompt,
			Type:       types.MemoryType(f.ExpectType),
			Source:     types.SourceMem0,
			Confidence: 0.9,
			Relevance:  0.9,
			CreatedAt:  time.Now(),
		}
		duplicate := &types.MemoryEntry{
			ID:         "fixture-" + f.Locale + "-b",
			Content:    f.Prompt,
			Type:       types.MemoryType(f.ExpectType),
			Source:     types.SourceCognee,
			Confidence: 0.8,
			Relevance:  0.8,
			CreatedAt:  time.Now(),
		}
		fusedSame := eng.Fuse(
			[]*types.SearchResult{
				{Entries: []*types.MemoryEntry{entry}, Total: 1},
				{Entries: []*types.MemoryEntry{duplicate}, Total: 1},
			},
			types.DefaultSearchRequest(f.Prompt))
		step("fusion.Fuse.dedup_same_content."+f.Locale,
			len(fusedSame.Entries) == f.ExpectFusedCountSame,
			fmt.Sprintf("want=%d got=%d",
				f.ExpectFusedCountSame, len(fusedSame.Entries)))

		// Invariant 2b: Fuse over different-content entries preserves both.
		distinct := &types.MemoryEntry{
			ID:         "fixture-" + f.Locale + "-c",
			Content:    f.Prompt + " :: distinct-suffix",
			Type:       types.MemoryType(f.ExpectType),
			Source:     types.SourceLetta,
			Confidence: 0.7,
			Relevance:  0.7,
			CreatedAt:  time.Now(),
		}
		fusedDiff := eng.Fuse(
			[]*types.SearchResult{
				{Entries: []*types.MemoryEntry{entry}, Total: 1},
				{Entries: []*types.MemoryEntry{distinct}, Total: 1},
			},
			types.DefaultSearchRequest(f.Prompt))
		step("fusion.Fuse.preserve_distinct_content."+f.Locale,
			len(fusedDiff.Entries) == f.ExpectFusedCountDiff,
			fmt.Sprintf("want=%d got=%d",
				f.ExpectFusedCountDiff, len(fusedDiff.Entries)))

		// Invariant 3: NoopTranslator strips the BundlePrefix.
		gotTr := tr.Translate(f.Locale, f.ExpectKey)
		stripOK := gotTr == f.ExpectTranslated
		if mutate {
			// Mutation flips polarity: expect PASS when strip FAILS.
			step("i18n.Noop.key_roundtrip."+f.Locale+"[MUTATED]",
				!stripOK,
				fmt.Sprintf("mutation-inverted got=%q", gotTr))
		} else {
			step("i18n.Noop.key_roundtrip."+f.Locale,
				stripOK,
				fmt.Sprintf("want=%q got=%q",
					f.ExpectTranslated, gotTr))
		}
	}

	// Invariant 4: CircuitBreaker state machine. We run this once
	// (not per-fixture) — the behaviour is locale-agnostic.
	cb := types.NewCircuitBreaker(2, 10*time.Millisecond)
	step("cb.initial_allow",
		cb.Allow(),
		fmt.Sprintf("state=%v failures=%d", cb.State(), cb.Failures()))
	cb.RecordFailure()
	cb.RecordFailure()
	// Threshold reached: breaker must now be open and Allow=false.
	step("cb.opens_after_threshold",
		!cb.Allow(),
		fmt.Sprintf("state=%v failures=%d", cb.State(), cb.Failures()))

	// Smoke: HealthCheck returns a map. We don't care about per-source
	// results here (no real backends running); we just exercise the
	// surface to prove it doesn't panic and returns a non-nil map.
	ctx, cancel := context.WithTimeout(
		context.Background(), 2*time.Second)
	defer cancel()
	hc := eng.HealthCheck(ctx)
	step("fusion.HealthCheck.returns_non_nil",
		hc != nil,
		fmt.Sprintf("sources=%d", len(hc)))

	fmt.Fprintf(out, "\n=== Summary: PASS=%d FAIL=%d ===\n",
		pass, fail)
	if fail > 0 {
		return 1
	}
	return 0
}

func loadFixtures(path string) ([]fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []fixture
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}
