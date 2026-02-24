package codebase_dna

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is a minimal in-memory MemoryProvider for unit testing.
type testProvider struct {
	name      types.MemorySource
	entries   map[string]*types.MemoryEntry
	healthy   bool
	addErr    error
	searchErr error
}

func newTestProvider(name types.MemorySource) *testProvider {
	return &testProvider{
		name:    name,
		entries: make(map[string]*types.MemoryEntry),
		healthy: true,
	}
}

func (p *testProvider) Name() types.MemorySource { return p.name }

func (p *testProvider) Add(_ context.Context, entry *types.MemoryEntry) error {
	if p.addErr != nil {
		return p.addErr
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Search(
	_ context.Context, req *types.SearchRequest,
) (*types.SearchResult, error) {
	if p.searchErr != nil {
		return nil, p.searchErr
	}
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		entries = append(entries, e)
		if len(entries) >= req.TopK {
			break
		}
	}
	return &types.SearchResult{
		Entries:  entries,
		Total:    len(entries),
		Duration: 1 * time.Millisecond,
		Sources:  []types.MemorySource{p.name},
	}, nil
}

func (p *testProvider) Get(_ context.Context, id string) (*types.MemoryEntry, error) {
	if e, ok := p.entries[id]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found")
}

func (p *testProvider) Update(_ context.Context, entry *types.MemoryEntry) error {
	if _, ok := p.entries[entry.ID]; !ok {
		return fmt.Errorf("not found")
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Delete(_ context.Context, id string) error {
	delete(p.entries, id)
	return nil
}

func (p *testProvider) GetHistory(
	_ context.Context, _ string, limit int,
) ([]*types.MemoryEntry, error) {
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		entries = append(entries, e)
		if len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

func (p *testProvider) Health(_ context.Context) error {
	if !p.healthy {
		return fmt.Errorf("unhealthy")
	}
	return nil
}

func TestProfiler_AnalyzeCode_Go(t *testing.T) {
	tests := []struct {
		name             string
		code             string
		language         string
		project          string
		expectedPatterns []string
	}{
		{
			name: "go code with interfaces, context, and error wrapping",
			code: `package main

import (
	"context"
	"fmt"
)

type Service interface {
	Do(ctx context.Context) error
}

func run(ctx context.Context) error {
	return fmt.Errorf("wrap: %w", err)
}
`,
			language: "go",
			project:  "test-project",
			expectedPatterns: []string{
				"interface_abstraction",
				"context_propagation",
				"error_wrapping",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			profiler := NewProfiler(prov)

			profile, err := profiler.AnalyzeCode(
				context.Background(), tc.code, tc.language, tc.project,
			)
			require.NoError(t, err)
			require.NotNil(t, profile)

			patternNames := make([]string, 0, len(profile.Patterns))
			for _, p := range profile.Patterns {
				patternNames = append(patternNames, p.Name)
			}

			for _, expected := range tc.expectedPatterns {
				assert.Contains(t, patternNames, expected,
					"expected pattern %q not found", expected)
			}

			// Verify entries stored in provider — one per detected pattern
			assert.Len(t, prov.entries, len(profile.Patterns))
		})
	}
}

func TestProfiler_AnalyzeCode_Python(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		expectPatterns []string
	}{
		{
			name: "python with ABC",
			code: `from abc import ABC, abstractmethod

class Handler(ABC):
    @abstractmethod
    def handle(self): pass
`,
			expectPatterns: []string{"abstract_base_classes"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			profiler := NewProfiler(prov)

			profile, err := profiler.AnalyzeCode(
				context.Background(), tc.code, "python", "py-proj",
			)
			require.NoError(t, err)

			patternNames := make([]string, 0, len(profile.Patterns))
			for _, p := range profile.Patterns {
				patternNames = append(patternNames, p.Name)
			}
			for _, expected := range tc.expectPatterns {
				assert.Contains(t, patternNames, expected)
			}
		})
	}
}

func TestProfiler_AnalyzeCode_Empty(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	profiler := NewProfiler(prov)

	profile, err := profiler.AnalyzeCode(
		context.Background(), "", "go", "empty-proj",
	)
	require.NoError(t, err)
	assert.Empty(t, profile.Patterns)
	assert.Len(t, prov.entries, 0)
}

func TestProfiler_GetProfile(t *testing.T) {
	tests := []struct {
		name            string
		seedEntries     []*types.MemoryEntry
		project         string
		expectPatterns  int
		expectLanguage  string
	}{
		{
			name: "profile reconstructed from stored entries",
			seedEntries: []*types.MemoryEntry{
				{
					ID:      "e1",
					Content: "[DNA:myproject] Pattern: error_wrapping — wraps",
					Metadata: map[string]interface{}{
						"dna_project":  "myproject",
						"dna_language": "go",
						"dna_type":     "pattern",
						"pattern_name": "error_wrapping",
					},
					Confidence: 0.9,
				},
				{
					ID:      "e2",
					Content: "[DNA:myproject] Pattern: context_propagation",
					Metadata: map[string]interface{}{
						"dna_project":  "myproject",
						"dna_language": "go",
						"dna_type":     "pattern",
						"pattern_name": "context_propagation",
					},
					Confidence: 0.95,
				},
			},
			project:        "myproject",
			expectPatterns: 2,
			expectLanguage: "go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for _, e := range tc.seedEntries {
				prov.entries[e.ID] = e
			}

			profiler := NewProfiler(prov)
			profile, err := profiler.GetProfile(context.Background(), tc.project)
			require.NoError(t, err)
			assert.Len(t, profile.Patterns, tc.expectPatterns)
			assert.Equal(t, tc.expectLanguage, profile.Language)
		})
	}
}

func TestProfiler_GetProfile_NotFound(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	profiler := NewProfiler(prov)

	_, err := profiler.GetProfile(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no profile found")
}

func TestProfiler_DetectPreferences(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		language string
		expectKV map[string]string
	}{
		{
			name:     "testify detected as testing preference",
			code:     `import "github.com/stretchr/testify/assert"`,
			language: "go",
			expectKV: map[string]string{
				"test_framework": "testify",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			profiler := NewProfiler(prov)

			profile, err := profiler.AnalyzeCode(
				context.Background(), tc.code, tc.language, "pref-proj",
			)
			require.NoError(t, err)

			prefMap := make(map[string]string)
			for _, pref := range profile.Preferences {
				prefMap[pref.Key] = pref.Value
			}
			for k, v := range tc.expectKV {
				assert.Equal(t, v, prefMap[k],
					"preference %q should be %q", k, v)
			}
		})
	}
}

func TestProfiler_DetectConventions(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		expectRule string
	}{
		{
			name:       "short lines detected as convention",
			code:       "package main\nfunc main() {}\n",
			expectRule: "Line length <= 100 characters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			profiler := NewProfiler(prov)

			profile, err := profiler.AnalyzeCode(
				context.Background(), tc.code, "go", "conv-proj",
			)
			require.NoError(t, err)

			var rules []string
			for _, c := range profile.Conventions {
				rules = append(rules, c.Rule)
			}
			assert.Contains(t, rules, tc.expectRule)
		})
	}
}

func TestProfiler_CalculateConfidence(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		language   string
		comparison string // "greater" or "less"
	}{
		{
			name: "large code with many patterns gets higher confidence",
			code: func() string {
				// Generate large code with multiple patterns
				base := `package main
import (
	"context"
	"fmt"
	"sync"
)
type Handler interface {
	Handle(ctx context.Context) error
}
func run(ctx context.Context) error {
	var mu sync.Mutex
	_ = mu
	return fmt.Errorf("error: %w", err)
}
`
				// Pad to many lines
				lines := base
				for i := 0; i < 200; i++ {
					lines += "// line padding\n"
				}
				return lines
			}(),
			language:   "go",
			comparison: "greater",
		},
		{
			name:       "small code gets lower confidence",
			code:       "package main\n",
			language:   "go",
			comparison: "less",
		},
	}

	prov := newTestProvider(types.SourceFusion)
	profiler := NewProfiler(prov)

	var scores []float64
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := profiler.AnalyzeCode(
				context.Background(), tc.code, tc.language, "conf-proj",
			)
			require.NoError(t, err)
			scores = append(scores, profile.ConfidenceScore)
		})
	}

	require.Len(t, scores, 2,
		"need exactly two scores for comparison")
	assert.Greater(t, scores[0], scores[1],
		"large code confidence should exceed small code confidence")
}

func TestProfiler_AnalyzeCode_AddError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.addErr = fmt.Errorf("storage failure")
	profiler := NewProfiler(prov)

	// Code that will detect at least one pattern so Add is attempted
	code := `package main
type Foo interface {
	Bar()
}
`
	_, err := profiler.AnalyzeCode(
		context.Background(), code, "go", "err-proj",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store pattern")
}
