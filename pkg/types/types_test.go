package types

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryType_Values(t *testing.T) {
	types := []MemoryType{
		MemoryTypeFact,
		MemoryTypeGraph,
		MemoryTypeCore,
		MemoryTypeTemporal,
		MemoryTypeEpisodic,
		MemoryTypeProcedural,
	}

	assert.Len(t, types, 6)
	assert.Equal(t, MemoryType("fact"), MemoryTypeFact)
	assert.Equal(t, MemoryType("graph"), MemoryTypeGraph)
	assert.Equal(t, MemoryType("core"), MemoryTypeCore)
	assert.Equal(t, MemoryType("temporal"), MemoryTypeTemporal)
	assert.Equal(t, MemoryType("episodic"), MemoryTypeEpisodic)
	assert.Equal(t, MemoryType("procedural"), MemoryTypeProcedural)
}

func TestMemorySource_Values(t *testing.T) {
	sources := []MemorySource{
		SourceMem0,
		SourceCognee,
		SourceLetta,
		SourceGraphiti,
		SourceFusion,
	}

	assert.Len(t, sources, 5)
	assert.Equal(t, MemorySource("mem0"), SourceMem0)
	assert.Equal(t, MemorySource("cognee"), SourceCognee)
	assert.Equal(t, MemorySource("letta"), SourceLetta)
	assert.Equal(t, MemorySource("graphiti"), SourceGraphiti)
	assert.Equal(t, MemorySource("fusion"), SourceFusion)
}

func TestMemoryEntry_Fields(t *testing.T) {
	now := time.Now()
	expires := now.Add(24 * time.Hour)

	entry := &MemoryEntry{
		ID:         "test-id",
		Content:    "test content",
		Type:       MemoryTypeFact,
		Source:     SourceMem0,
		Confidence: 0.95,
		Relevance:  0.85,
		Metadata:   map[string]interface{}{"key": "value"},
		Embedding:  []float32{0.1, 0.2, 0.3},
		UserID:     "user-1",
		SessionID:  "session-1",
		AgentID:    "agent-1",
		Tags:       []string{"tag1", "tag2"},
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  &expires,
		AccessCount: 5,
		LastAccess:  now,
	}

	assert.Equal(t, "test-id", entry.ID)
	assert.Equal(t, "test content", entry.Content)
	assert.Equal(t, MemoryTypeFact, entry.Type)
	assert.Equal(t, SourceMem0, entry.Source)
	assert.InDelta(t, 0.95, entry.Confidence, 0.01)
	assert.InDelta(t, 0.85, entry.Relevance, 0.01)
	assert.Equal(t, "value", entry.Metadata["key"])
	assert.Len(t, entry.Embedding, 3)
	assert.Equal(t, "user-1", entry.UserID)
	assert.Equal(t, "session-1", entry.SessionID)
	assert.Equal(t, "agent-1", entry.AgentID)
	assert.Len(t, entry.Tags, 2)
	assert.Equal(t, now, entry.CreatedAt)
	require.NotNil(t, entry.ExpiresAt)
	assert.Equal(t, expires, *entry.ExpiresAt)
	assert.Equal(t, 5, entry.AccessCount)
}

func TestCoreMemoryBlock(t *testing.T) {
	block := &CoreMemoryBlock{
		Label:   "persona",
		Value:   "I am HelixMemory",
		Limit:   5000,
		AgentID: "agent-1",
	}

	assert.Equal(t, "persona", block.Label)
	assert.Equal(t, "I am HelixMemory", block.Value)
	assert.Equal(t, 5000, block.Limit)
	assert.Equal(t, "agent-1", block.AgentID)
}

func TestSearchRequest_Defaults(t *testing.T) {
	req := DefaultSearchRequest("test query")

	assert.Equal(t, "test query", req.Query)
	assert.Equal(t, 10, req.TopK)
	assert.InDelta(t, 0.0, req.MinScore, 0.001)
	assert.Empty(t, req.UserID)
	assert.Empty(t, req.SessionID)
	assert.Nil(t, req.TimeRange)
	assert.Nil(t, req.Filter)
}

func TestSearchRequest_WithOptions(t *testing.T) {
	now := time.Now()
	req := &SearchRequest{
		Query:     "test",
		UserID:    "user-1",
		SessionID: "session-1",
		AgentID:   "agent-1",
		Types:     []MemoryType{MemoryTypeFact, MemoryTypeGraph},
		Sources:   []MemorySource{SourceMem0, SourceCognee},
		TopK:      5,
		MinScore:  0.5,
		TimeRange: &TimeRange{Start: now.Add(-1 * time.Hour), End: now},
		Filter:    map[string]interface{}{"category": "code"},
	}

	assert.Equal(t, "test", req.Query)
	assert.Equal(t, "user-1", req.UserID)
	assert.Len(t, req.Types, 2)
	assert.Len(t, req.Sources, 2)
	assert.Equal(t, 5, req.TopK)
	assert.InDelta(t, 0.5, req.MinScore, 0.01)
	require.NotNil(t, req.TimeRange)
	assert.Equal(t, "code", req.Filter["category"])
}

func TestSearchResult(t *testing.T) {
	result := &SearchResult{
		Entries: []*MemoryEntry{
			{ID: "1", Content: "test1"},
			{ID: "2", Content: "test2"},
		},
		Total:    2,
		Duration: 100 * time.Millisecond,
		Sources:  []MemorySource{SourceMem0, SourceCognee},
	}

	assert.Len(t, result.Entries, 2)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 100*time.Millisecond, result.Duration)
	assert.Len(t, result.Sources, 2)
}

func TestConsolidationStatus(t *testing.T) {
	status := &ConsolidationStatus{
		Running:           true,
		LastRun:           time.Now(),
		MemoriesProcessed: 100,
		Deduplicated:      10,
		Consolidated:      90,
		Duration:          5 * time.Second,
	}

	assert.True(t, status.Running)
	assert.Equal(t, 100, status.MemoriesProcessed)
	assert.Equal(t, 10, status.Deduplicated)
	assert.Equal(t, 90, status.Consolidated)
	assert.Equal(t, 5*time.Second, status.Duration)
}

// TestMemoryProvider_Interface verifies the interface contract.
func TestMemoryProvider_Interface(t *testing.T) {
	// Compile-time check that mock implements the interface
	var _ MemoryProvider = (*mockProvider)(nil)
	var _ CoreMemoryProvider = (*mockCoreProvider)(nil)
	var _ ConsolidationProvider = (*mockConsolidationProvider)(nil)
	var _ TemporalProvider = (*mockTemporalProvider)(nil)
}

type mockProvider struct{}

func (m *mockProvider) Name() MemorySource                                                     { return SourceMem0 }
func (m *mockProvider) Add(context.Context, *MemoryEntry) error                                { return nil }
func (m *mockProvider) Search(context.Context, *SearchRequest) (*SearchResult, error)           { return nil, nil }
func (m *mockProvider) Get(context.Context, string) (*MemoryEntry, error)                       { return nil, nil }
func (m *mockProvider) Update(context.Context, *MemoryEntry) error                              { return nil }
func (m *mockProvider) Delete(context.Context, string) error                                    { return nil }
func (m *mockProvider) GetHistory(context.Context, string, int) ([]*MemoryEntry, error)         { return nil, nil }
func (m *mockProvider) Health(context.Context) error                                            { return nil }

type mockCoreProvider struct{ mockProvider }

func (m *mockCoreProvider) GetCoreMemory(context.Context, string) ([]*CoreMemoryBlock, error) { return nil, nil }
func (m *mockCoreProvider) UpdateCoreMemory(context.Context, string, *CoreMemoryBlock) error  { return nil }

type mockConsolidationProvider struct{ mockProvider }

func (m *mockConsolidationProvider) TriggerConsolidation(context.Context, string) error               { return nil }
func (m *mockConsolidationProvider) GetConsolidationStatus(context.Context) (*ConsolidationStatus, error) { return nil, nil }

type mockTemporalProvider struct{ mockProvider }

func (m *mockTemporalProvider) SearchTemporal(context.Context, string, time.Time) ([]*MemoryEntry, error) { return nil, nil }
func (m *mockTemporalProvider) GetTimeline(context.Context, string, time.Time, time.Time) ([]*MemoryEntry, error) { return nil, nil }
func (m *mockTemporalProvider) InvalidateAt(context.Context, string, time.Time) error { return nil }
