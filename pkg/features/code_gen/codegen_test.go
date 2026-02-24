package code_gen

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is an in-memory mock of types.MemoryProvider for unit tests.
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
		if req.TopK > 0 && len(entries) >= req.TopK {
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

func TestGenerator_GetCodeContext(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	// Populate with pattern, convention, and example entries.
	// The provider returns all entries for any search, so each
	// category call picks them up.
	prov.entries["p1"] = &types.MemoryEntry{
		ID:      "p1",
		Content: "singleton pattern implementation",
		Type:    types.MemoryTypeProcedural,
	}
	prov.entries["c1"] = &types.MemoryEntry{
		ID:      "c1",
		Content: "use camelCase for private functions",
		Type:    types.MemoryTypeFact,
	}
	prov.entries["ex1"] = &types.MemoryEntry{
		ID:      "ex1",
		Content: "func NewService() *Service { return &Service{} }",
		Type:    types.MemoryTypeProcedural,
	}

	gen := NewGenerator(prov)
	ctx := context.Background()

	codeCtx, err := gen.GetCodeContext(ctx, "create a service", "Go", "myproject")
	require.NoError(t, err)

	// All entries are returned for every search call, so each
	// slice should be populated.
	assert.NotEmpty(t, codeCtx.Patterns, "Patterns should be populated")
	assert.NotEmpty(t, codeCtx.Conventions, "Conventions should be populated")
	assert.NotEmpty(t, codeCtx.Examples, "Examples should be populated")

	// Verify the prompt contains all expected sections
	prompt := codeCtx.Prompt
	assert.Contains(t, prompt, "Task:")
	assert.Contains(t, prompt, "Language:")
	assert.Contains(t, prompt, "Project:")
	assert.Contains(t, prompt, "Known Patterns:")
	assert.Contains(t, prompt, "Conventions:")
	assert.Contains(t, prompt, "Related Examples:")
}

func TestGenerator_GetCodeContext_Empty(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	// No entries in provider

	gen := NewGenerator(prov)
	ctx := context.Background()

	codeCtx, err := gen.GetCodeContext(ctx, "create a service", "Go", "emptyproject")
	require.NoError(t, err)

	assert.Empty(t, codeCtx.Patterns)
	assert.Empty(t, codeCtx.Conventions)
	assert.Empty(t, codeCtx.Examples)

	// Prompt should still contain task/language/project but no section headers
	assert.Contains(t, codeCtx.Prompt, "Task:")
	assert.Contains(t, codeCtx.Prompt, "Language:")
	assert.Contains(t, codeCtx.Prompt, "Project:")
	assert.NotContains(t, codeCtx.Prompt, "Known Patterns:")
	assert.NotContains(t, codeCtx.Prompt, "Conventions:")
	assert.NotContains(t, codeCtx.Prompt, "Related Examples:")
}

func TestGenerator_BuildPrompt_AllSections(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	gen := NewGenerator(prov)

	codeCtx := &CodeContext{
		Patterns: []*types.MemoryEntry{
			{Content: "observer pattern"},
		},
		Conventions: []*types.MemoryEntry{
			{Content: "use PascalCase for exports"},
		},
		Examples: []*types.MemoryEntry{
			{Content: "func main() {}"},
		},
	}

	prompt := gen.buildPrompt("implement caching", "Go", "myproject", codeCtx)

	assert.Contains(t, prompt, "Task: implement caching")
	assert.Contains(t, prompt, "Language: Go")
	assert.Contains(t, prompt, "Project: myproject")
	assert.Contains(t, prompt, "Known Patterns:")
	assert.Contains(t, prompt, "observer pattern")
	assert.Contains(t, prompt, "Conventions:")
	assert.Contains(t, prompt, "PascalCase")
	assert.Contains(t, prompt, "Related Examples:")
	assert.Contains(t, prompt, "func main()")
}

func TestGenerator_BuildPrompt_NoPatterns(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	gen := NewGenerator(prov)

	codeCtx := &CodeContext{
		Patterns:    nil,
		Conventions: nil,
		Examples: []*types.MemoryEntry{
			{Content: "example code snippet"},
		},
	}

	prompt := gen.buildPrompt("write tests", "Go", "proj", codeCtx)

	assert.NotContains(t, prompt, "Known Patterns:")
	assert.NotContains(t, prompt, "Conventions:")
	assert.Contains(t, prompt, "Related Examples:")
	assert.Contains(t, prompt, "example code snippet")
}

func TestGenerator_Truncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long string truncated with ellipsis",
			input:    "this is a very long string that exceeds the limit",
			maxLen:   10,
			expected: "this is a " + "...",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := truncate(tc.input, tc.maxLen)
			assert.Equal(t, tc.expected, result)

			if len(tc.input) > tc.maxLen {
				assert.True(t, strings.HasSuffix(result, "..."))
				assert.Equal(t, tc.maxLen+3, len(result))
			}
		})
	}
}
