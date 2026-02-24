package context_window

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

func TestContextWindow_Build(t *testing.T) {
	tests := []struct {
		name          string
		maxTokens     int
		entries       map[string]*types.MemoryEntry
		expectBlocks  int
		expectDropped int
	}{
		{
			name:      "entries packed by priority into context window",
			maxTokens: 100000,
			entries: map[string]*types.MemoryEntry{
				"e1": {
					ID:         "e1",
					Content:    "Important fact about Go concurrency",
					Source:     types.SourceFusion,
					Relevance:  0.9,
					Confidence: 0.85,
					Metadata:   map[string]interface{}{},
				},
				"e2": {
					ID:         "e2",
					Content:    "Another relevant memory",
					Source:     types.SourceMem0,
					Relevance:  0.7,
					Confidence: 0.6,
					Metadata:   map[string]interface{}{},
				},
			},
			expectBlocks:  2,
			expectDropped: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for k, v := range tc.entries {
				prov.entries[k] = v
			}

			cw := NewContextWindow(prov, tc.maxTokens)
			managed, err := cw.Build(
				context.Background(), "concurrency", "user-1",
			)
			require.NoError(t, err)
			require.NotNil(t, managed)

			assert.Len(t, managed.Blocks, tc.expectBlocks)
			assert.Equal(t, tc.expectDropped, managed.Dropped)
			assert.Greater(t, managed.TotalTokens, 0)
			assert.Greater(t, managed.Utilization, 0.0)
			assert.Equal(t, tc.maxTokens, managed.MaxTokens)
		})
	}
}

func TestContextWindow_Build_ExceedsTokenBudget(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		reserved  int
	}{
		{
			name:      "entries exceed budget, some dropped",
			maxTokens: 100, // very small budget
			reserved:  50,  // leaves only 50 tokens available
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)

			// Add entries whose combined token estimate exceeds budget
			longContent := strings.Repeat("word ", 200) // large content
			for i := 0; i < 5; i++ {
				id := fmt.Sprintf("big-%d", i)
				prov.entries[id] = &types.MemoryEntry{
					ID:         id,
					Content:    longContent,
					Source:     types.SourceFusion,
					Relevance:  0.8,
					Confidence: 0.7,
					Metadata:   map[string]interface{}{},
				}
			}

			cw := NewContextWindow(prov, tc.maxTokens)
			cw.SetReserved(tc.reserved)

			managed, err := cw.Build(
				context.Background(), "query", "user-1",
			)
			require.NoError(t, err)

			assert.Greater(t, managed.Dropped, 0,
				"some entries should be dropped")
			available := tc.maxTokens - tc.reserved
			assert.LessOrEqual(t, managed.TotalTokens, available,
				"total tokens should not exceed available budget")
		})
	}
}

func TestContextWindow_Build_SearchError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.searchErr = fmt.Errorf("connection refused")

	cw := NewContextWindow(prov, 8000)
	_, err := cw.Build(context.Background(), "query", "user-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context window search")
}

func TestContextWindow_Build_EmptyResults(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	cw := NewContextWindow(prov, 8000)

	managed, err := cw.Build(
		context.Background(), "nothing here", "user-1",
	)
	require.NoError(t, err)
	assert.Empty(t, managed.Blocks)
	assert.Equal(t, 0, managed.TotalTokens)
	assert.Equal(t, 0.0, managed.Utilization)
}

func TestContextWindow_SetReserved(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		reserved  int
	}{
		{
			name:      "set reserved tokens affects available budget",
			maxTokens: 10000,
			reserved:  5000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)

			// Add a moderately sized entry
			content := strings.Repeat("some text here ", 50)
			prov.entries["e1"] = &types.MemoryEntry{
				ID:         "e1",
				Content:    content,
				Source:     types.SourceFusion,
				Relevance:  0.9,
				Confidence: 0.9,
				Metadata:   map[string]interface{}{},
			}

			cw := NewContextWindow(prov, tc.maxTokens)
			cw.SetReserved(tc.reserved)

			managed, err := cw.Build(
				context.Background(), "query", "user-1",
			)
			require.NoError(t, err)

			available := tc.maxTokens - tc.reserved
			assert.LessOrEqual(t, managed.TotalTokens, available,
				"total tokens must respect reserved budget")
		})
	}
}

func TestContextWindow_EstimateTokens(t *testing.T) {
	// estimateTokens is private, test indirectly through Build
	tests := []struct {
		name          string
		contentSize   int // approximate word count
		expectMinTok  int
		expectMaxTok  int
	}{
		{
			name:         "short content produces small token count",
			contentSize:  5,
			expectMinTok: 1,
			expectMaxTok: 50,
		},
		{
			name:         "long content produces larger token count",
			contentSize:  500,
			expectMinTok: 50,
			expectMaxTok: 5000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)

			content := strings.Repeat("word ", tc.contentSize)
			prov.entries["tok-test"] = &types.MemoryEntry{
				ID:         "tok-test",
				Content:    content,
				Source:     types.SourceFusion,
				Relevance:  0.5,
				Confidence: 0.5,
				Metadata:   map[string]interface{}{},
			}

			cw := NewContextWindow(prov, 1000000) // large budget
			managed, err := cw.Build(
				context.Background(), "query", "user-1",
			)
			require.NoError(t, err)
			require.Len(t, managed.Blocks, 1)

			tokens := managed.Blocks[0].TokenCount
			assert.GreaterOrEqual(t, tokens, tc.expectMinTok)
			assert.LessOrEqual(t, tokens, tc.expectMaxTok)
		})
	}
}

func TestContextWindow_Priority(t *testing.T) {
	tests := []struct {
		name         string
		entries      []*types.MemoryEntry
		expectFirst  string // ID of highest-priority entry
	}{
		{
			name: "higher priority entries packed first",
			entries: []*types.MemoryEntry{
				{
					ID:         "low",
					Content:    "low priority content",
					Source:     types.SourceFusion,
					Relevance:  0.3,
					Confidence: 0.2,
					Metadata:   map[string]interface{}{},
				},
				{
					ID:         "high",
					Content:    "high priority content",
					Source:     types.SourceFusion,
					Relevance:  0.9,
					Confidence: 0.95,
					Metadata:   map[string]interface{}{},
				},
			},
			// priority = relevance*0.6 + confidence*0.4
			// high: 0.9*0.6 + 0.95*0.4 = 0.54 + 0.38 = 0.92
			// low:  0.3*0.6 + 0.2*0.4  = 0.18 + 0.08 = 0.26
			expectFirst: "high",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for _, e := range tc.entries {
				prov.entries[e.ID] = e
			}

			cw := NewContextWindow(prov, 100000)
			managed, err := cw.Build(
				context.Background(), "query", "user-1",
			)
			require.NoError(t, err)
			require.NotEmpty(t, managed.Blocks)

			// The first block should have the highest priority
			firstBlock := managed.Blocks[0]

			// Verify the first block matches the expected high-priority
			// content by checking the priority score
			highPriority := 0.9*0.6 + 0.95*0.4
			lowPriority := 0.3*0.6 + 0.2*0.4
			assert.Greater(t, firstBlock.Priority, lowPriority)
			assert.InDelta(t, highPriority, firstBlock.Priority, 0.01)
		})
	}
}
