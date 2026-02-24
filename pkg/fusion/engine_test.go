package fusion

import (
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEngine() *Engine {
	return NewEngine(config.DefaultConfig())
}

func TestEngine_Fuse_EmptyResults(t *testing.T) {
	e := newTestEngine()
	req := types.DefaultSearchRequest("test")

	result := e.Fuse(nil, req)
	assert.NotNil(t, result)
	assert.Empty(t, result.Entries)
}

func TestEngine_Fuse_SingleSource(t *testing.T) {
	e := newTestEngine()
	req := types.DefaultSearchRequest("test")

	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{ID: "1", Content: "test 1", Relevance: 0.9, Source: types.SourceMem0, CreatedAt: time.Now()},
				{ID: "2", Content: "test 2", Relevance: 0.8, Source: types.SourceMem0, CreatedAt: time.Now()},
			},
			Total:   2,
			Sources: []types.MemorySource{types.SourceMem0},
		},
	}

	result := e.Fuse(results, req)
	assert.NotNil(t, result)
	assert.Len(t, result.Entries, 2)
	assert.Contains(t, result.Sources, types.SourceMem0)
}

func TestEngine_Fuse_MultiSource(t *testing.T) {
	e := newTestEngine()
	req := types.DefaultSearchRequest("test")

	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{ID: "1", Content: "result from mem0", Relevance: 0.9, Source: types.SourceMem0, Type: types.MemoryTypeFact, CreatedAt: time.Now()},
			},
			Sources: []types.MemorySource{types.SourceMem0},
		},
		{
			Entries: []*types.MemoryEntry{
				{ID: "2", Content: "result from cognee", Relevance: 0.85, Source: types.SourceCognee, Type: types.MemoryTypeGraph, CreatedAt: time.Now()},
			},
			Sources: []types.MemorySource{types.SourceCognee},
		},
		{
			Entries: []*types.MemoryEntry{
				{ID: "3", Content: "result from letta", Relevance: 0.95, Source: types.SourceLetta, Type: types.MemoryTypeCore, CreatedAt: time.Now()},
			},
			Sources: []types.MemorySource{types.SourceLetta},
		},
	}

	result := e.Fuse(results, req)
	assert.NotNil(t, result)
	assert.Len(t, result.Entries, 3)
	assert.Len(t, result.Sources, 3)
}

func TestEngine_Fuse_Deduplication(t *testing.T) {
	e := newTestEngine()
	req := types.DefaultSearchRequest("test")

	// Two identical entries from different sources
	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{ID: "1", Content: "the exact same content", Relevance: 0.9, Source: types.SourceMem0, Type: types.MemoryTypeFact, CreatedAt: time.Now(), Confidence: 0.8},
			},
			Sources: []types.MemorySource{types.SourceMem0},
		},
		{
			Entries: []*types.MemoryEntry{
				{ID: "2", Content: "the exact same content", Relevance: 0.85, Source: types.SourceCognee, Type: types.MemoryTypeGraph, CreatedAt: time.Now(), Confidence: 0.9},
			},
			Sources: []types.MemorySource{types.SourceCognee},
		},
	}

	result := e.Fuse(results, req)
	assert.NotNil(t, result)
	// Should deduplicate identical content
	assert.Equal(t, 1, len(result.Entries))
	// Should keep the one with higher confidence
	assert.InDelta(t, 0.9, result.Entries[0].Confidence, 0.01)
}

func TestEngine_Fuse_TopKLimit(t *testing.T) {
	e := newTestEngine()
	req := &types.SearchRequest{Query: "test", TopK: 2}

	entries := make([]*types.MemoryEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = &types.MemoryEntry{
			ID:        string(rune('a' + i)),
			Content:   "different content " + string(rune('a'+i)),
			Relevance: float64(10-i) / 10.0,
			Source:    types.SourceMem0,
			Type:      types.MemoryTypeFact,
			CreatedAt: time.Now(),
		}
	}

	results := []*types.SearchResult{{Entries: entries, Sources: []types.MemorySource{types.SourceMem0}}}

	result := e.Fuse(results, req)
	assert.Len(t, result.Entries, 2)
}

func TestEngine_Fuse_NilResults(t *testing.T) {
	e := newTestEngine()
	req := types.DefaultSearchRequest("test")

	results := []*types.SearchResult{nil, nil}

	result := e.Fuse(results, req)
	assert.NotNil(t, result)
	assert.Empty(t, result.Entries)
}

func TestEngine_Reranking_TypeBoost(t *testing.T) {
	e := newTestEngine()
	req := &types.SearchRequest{
		Query: "test",
		TopK:  10,
		Types: []types.MemoryType{types.MemoryTypeFact},
	}

	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{ID: "1", Content: "fact entry A", Relevance: 0.7, Source: types.SourceMem0, Type: types.MemoryTypeFact, CreatedAt: time.Now()},
				{ID: "2", Content: "graph entry B", Relevance: 0.8, Source: types.SourceCognee, Type: types.MemoryTypeGraph, CreatedAt: time.Now()},
			},
			Sources: []types.MemorySource{types.SourceMem0, types.SourceCognee},
		},
	}

	result := e.Fuse(results, req)
	require.Len(t, result.Entries, 2)
	// Fact should be boosted because Types filter was specified
	assert.Equal(t, types.MemoryTypeFact, result.Entries[0].Type)
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float64
		delta    float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			delta:    0.001,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "zero vector",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 1, 1},
			expected: 0.0,
			delta:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected float64
		delta    float64
	}{
		{
			name:     "identical",
			a:        "hello world",
			b:        "hello world",
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "completely different",
			a:        "hello world",
			b:        "foo bar",
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "partial overlap",
			a:        "hello world foo",
			b:        "hello world bar",
			expected: 0.5,
			delta:    0.001,
		},
		{
			name:     "empty strings",
			a:        "",
			b:        "",
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "one empty",
			a:        "hello",
			b:        "",
			expected: 0.0,
			delta:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jaccardSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple words",
			input:    "hello world",
			expected: []string{"hello", "world"},
		},
		{
			name:     "mixed case",
			input:    "Hello World",
			expected: []string{"hello", "world"},
		},
		{
			name:     "with punctuation",
			input:    "hello, world! foo-bar.",
			expected: []string{"hello", "world", "foo", "bar"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "numbers",
			input:    "test123 hello456",
			expected: []string{"test123", "hello456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEngine_RecencyScore(t *testing.T) {
	e := newTestEngine()
	now := time.Now()

	// Very recent should be close to 1.0
	score := e.recencyScore(now, now)
	assert.InDelta(t, 1.0, score, 0.01)

	// 1 day old should be less
	score = e.recencyScore(now.Add(-24*time.Hour), now)
	assert.Less(t, score, 1.0)
	assert.Greater(t, score, 0.5)

	// 30 days old should be much less
	score = e.recencyScore(now.Add(-30*24*time.Hour), now)
	assert.Less(t, score, 0.5)
}

func TestEngine_SourceScore(t *testing.T) {
	e := newTestEngine()

	assert.InDelta(t, 0.95, e.sourceScore(types.SourceLetta), 0.01)
	assert.InDelta(t, 0.85, e.sourceScore(types.SourceMem0), 0.01)
	assert.InDelta(t, 0.80, e.sourceScore(types.SourceCognee), 0.01)
	assert.InDelta(t, 0.85, e.sourceScore(types.SourceGraphiti), 0.01)
	assert.InDelta(t, 0.90, e.sourceScore(types.SourceFusion), 0.01)
	assert.InDelta(t, 0.50, e.sourceScore(types.MemorySource("unknown")), 0.01)
}
