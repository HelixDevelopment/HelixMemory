package integration

import (
	"fmt"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/fusion"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFusionEngine() *fusion.Engine {
	return fusion.NewEngine(config.DefaultConfig())
}

func TestFusionEngine_Deduplication(t *testing.T) {
	e := newFusionEngine()
	req := types.DefaultSearchRequest("test")

	now := time.Now()

	// Two entries with identical content from different sources.
	// Jaccard similarity of identical strings is 1.0 (>= 0.92 threshold),
	// so they should be merged.
	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "dup-1", Content: "the exact same content here",
					Source: types.SourceMem0, Type: types.MemoryTypeFact,
					Relevance: 0.9, Confidence: 0.7, CreatedAt: now,
				},
			},
			Sources: []types.MemorySource{types.SourceMem0},
		},
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "dup-2", Content: "the exact same content here",
					Source: types.SourceCognee, Type: types.MemoryTypeGraph,
					Relevance: 0.85, Confidence: 0.95, CreatedAt: now,
				},
			},
			Sources: []types.MemorySource{types.SourceCognee},
		},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.Equal(t, 1, len(result.Entries),
		"identical content should be deduplicated to a single entry")
	// Should keep the entry with higher confidence (0.95)
	assert.InDelta(t, 0.95, result.Entries[0].Confidence, 0.01,
		"should keep entry with higher confidence")
}

func TestFusionEngine_DeduplicationWithEmbeddings(t *testing.T) {
	e := newFusionEngine()
	req := types.DefaultSearchRequest("test")

	now := time.Now()

	// Two entries with near-identical embeddings (cosine sim > 0.92)
	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "emb-1", Content: "content A",
					Source: types.SourceMem0, Type: types.MemoryTypeFact,
					Relevance: 0.9, Confidence: 0.6,
					Embedding: []float32{0.9, 0.1, 0.0},
					CreatedAt: now,
				},
			},
			Sources: []types.MemorySource{types.SourceMem0},
		},
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "emb-2", Content: "content B different text",
					Source: types.SourceCognee, Type: types.MemoryTypeGraph,
					Relevance: 0.85, Confidence: 0.8,
					// Very similar embedding direction
					Embedding: []float32{0.91, 0.09, 0.01},
					CreatedAt: now,
				},
			},
			Sources: []types.MemorySource{types.SourceCognee},
		},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.Equal(t, 1, len(result.Entries),
		"entries with similar embeddings (cosine >= 0.92) should be deduplicated")
}

func TestFusionEngine_CrossSourceRanking(t *testing.T) {
	e := newFusionEngine()
	now := time.Now()

	req := &types.SearchRequest{
		Query: "test",
		TopK:  10,
		Types: []types.MemoryType{types.MemoryTypeCore},
	}

	// Three entries from different sources with different properties
	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "rank-1", Content: "mem0 entry ranked",
					Source: types.SourceMem0, Type: types.MemoryTypeFact,
					Relevance: 0.7, CreatedAt: now.Add(-48 * time.Hour),
				},
			},
			Sources: []types.MemorySource{types.SourceMem0},
		},
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "rank-2", Content: "letta core entry ranked",
					Source: types.SourceLetta, Type: types.MemoryTypeCore,
					Relevance: 0.6, CreatedAt: now,
				},
			},
			Sources: []types.MemorySource{types.SourceLetta},
		},
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "rank-3", Content: "cognee graph entry ranked",
					Source: types.SourceCognee, Type: types.MemoryTypeGraph,
					Relevance: 0.8, CreatedAt: now.Add(-24 * time.Hour),
				},
			},
			Sources: []types.MemorySource{types.SourceCognee},
		},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.Len(t, result.Entries, 3)

	// The core entry (letta) should rank highest since
	// Types filter = [core], giving it type score 1.0 vs 0.3 for others.
	// Combined with Letta's high source score (0.95) and being most recent.
	assert.Equal(t, types.MemoryTypeCore, result.Entries[0].Type,
		"core type should rank first when requested in Types filter")
}

func TestFusionEngine_EmptyResults(t *testing.T) {
	e := newFusionEngine()
	req := types.DefaultSearchRequest("test")

	// nil input
	result := e.Fuse(nil, req)
	require.NotNil(t, result)
	assert.Empty(t, result.Entries, "nil results should produce empty entries")

	// Empty slice
	result = e.Fuse([]*types.SearchResult{}, req)
	require.NotNil(t, result)
	assert.Empty(t, result.Entries)

	// Slice with nil elements
	result = e.Fuse([]*types.SearchResult{nil, nil, nil}, req)
	require.NotNil(t, result)
	assert.Empty(t, result.Entries)

	// Result with empty entries
	result = e.Fuse([]*types.SearchResult{
		{Entries: []*types.MemoryEntry{}, Total: 0},
	}, req)
	require.NotNil(t, result)
	assert.Empty(t, result.Entries)
}

func TestFusionEngine_SingleSource(t *testing.T) {
	e := newFusionEngine()
	req := types.DefaultSearchRequest("test")
	now := time.Now()

	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "single-1", Content: "only source entry A",
					Source: types.SourceMem0, Type: types.MemoryTypeFact,
					Relevance: 0.9, CreatedAt: now,
				},
				{
					ID: "single-2", Content: "only source entry B",
					Source: types.SourceMem0, Type: types.MemoryTypeFact,
					Relevance: 0.8, CreatedAt: now.Add(-time.Hour),
				},
			},
			Total:   2,
			Sources: []types.MemorySource{types.SourceMem0},
		},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.Len(t, result.Entries, 2,
		"single source should pass through both entries")
	assert.Contains(t, result.Sources, types.SourceMem0)
	assert.Len(t, result.Sources, 1)
}

func TestFusionEngine_MixedTypes(t *testing.T) {
	e := newFusionEngine()
	req := types.DefaultSearchRequest("test")
	now := time.Now()

	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "mix-fact", Content: "a fact entry",
					Source: types.SourceMem0, Type: types.MemoryTypeFact,
					Relevance: 0.85, CreatedAt: now,
				},
				{
					ID: "mix-graph", Content: "a graph entry different",
					Source: types.SourceCognee, Type: types.MemoryTypeGraph,
					Relevance: 0.85, CreatedAt: now,
				},
				{
					ID: "mix-core", Content: "a core entry unique",
					Source: types.SourceLetta, Type: types.MemoryTypeCore,
					Relevance: 0.85, CreatedAt: now,
				},
				{
					ID: "mix-temporal", Content: "a temporal entry distinct",
					Source:    types.SourceGraphiti,
					Type:      types.MemoryTypeTemporal,
					Relevance: 0.85, CreatedAt: now,
				},
				{
					ID: "mix-episodic", Content: "an episodic entry special",
					Source: types.SourceLetta, Type: types.MemoryTypeEpisodic,
					Relevance: 0.85, CreatedAt: now,
				},
				{
					ID: "mix-proc", Content: "a procedural entry original",
					Source:    types.SourceCognee,
					Type:      types.MemoryTypeProcedural,
					Relevance: 0.85, CreatedAt: now,
				},
			},
			Sources: []types.MemorySource{
				types.SourceMem0, types.SourceCognee,
				types.SourceLetta, types.SourceGraphiti,
			},
		},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.Len(t, result.Entries, 6,
		"all different types should be preserved")

	// With no type filter, default type scores apply:
	// core (0.90) > fact/procedural (0.85) > graph (0.80) >
	// temporal (0.75) > episodic (0.70)
	// Combined with source scores, core from Letta (0.95) should rank highest.
	assert.Equal(t, types.MemoryTypeCore, result.Entries[0].Type,
		"core type should rank highest with default scores")
}

func TestFusionEngine_TopKRespected(t *testing.T) {
	e := newFusionEngine()
	now := time.Now()

	req := &types.SearchRequest{Query: "test", TopK: 3}

	entries := make([]*types.MemoryEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = &types.MemoryEntry{
			ID:        fmt.Sprintf("topk-%d", i),
			Content:   fmt.Sprintf("unique content %d for topk", i),
			Relevance: float64(10-i) / 10.0,
			Source:    types.SourceMem0,
			Type:      types.MemoryTypeFact,
			CreatedAt: now,
		}
	}

	results := []*types.SearchResult{
		{Entries: entries, Sources: []types.MemorySource{types.SourceMem0}},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.LessOrEqual(t, len(result.Entries), 3,
		"should respect TopK limit")
}

func TestFusionEngine_DurationAggregation(t *testing.T) {
	e := newFusionEngine()
	req := types.DefaultSearchRequest("test")
	now := time.Now()

	results := []*types.SearchResult{
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "dur-1", Content: "entry one for duration",
					Source: types.SourceMem0, CreatedAt: now,
				},
			},
			Duration: 50 * time.Millisecond,
			Sources:  []types.MemorySource{types.SourceMem0},
		},
		{
			Entries: []*types.MemoryEntry{
				{
					ID: "dur-2", Content: "entry two for duration",
					Source: types.SourceCognee, CreatedAt: now,
				},
			},
			Duration: 30 * time.Millisecond,
			Sources:  []types.MemorySource{types.SourceCognee},
		},
	}

	result := e.Fuse(results, req)
	require.NotNil(t, result)
	assert.Equal(t, 80*time.Millisecond, result.Duration,
		"duration should be sum of all source durations")
}
