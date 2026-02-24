package provider

import (
	"context"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	modstore "digital.vasic.memory/pkg/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdapter() (*MemoryStoreAdapter, *testProvider) {
	cfg := config.DefaultConfig()
	u := New(cfg)
	mem0 := newTestProvider(types.SourceMem0)
	u.RegisterProvider(mem0)
	return NewMemoryStoreAdapter(u), mem0
}

func TestMemoryStoreAdapter_Add(t *testing.T) {
	adapter, mem0 := newTestAdapter()
	ctx := context.Background()

	mem := &modstore.Memory{
		ID:      "test-1",
		Content: "test content",
		Scope:   modstore.ScopeUser,
		Metadata: map[string]any{
			"key": "value",
		},
	}

	err := adapter.Add(ctx, mem)
	assert.NoError(t, err)
	assert.Len(t, mem0.entries, 1)
}

func TestMemoryStoreAdapter_Search(t *testing.T) {
	adapter, mem0 := newTestAdapter()
	ctx := context.Background()

	mem0.entries["1"] = &types.MemoryEntry{
		ID:        "1",
		Content:   "search result",
		Source:    types.SourceMem0,
		Type:      types.MemoryTypeFact,
		CreatedAt: time.Now(),
	}

	opts := &modstore.SearchOptions{TopK: 10}
	results, err := adapter.Search(ctx, "test", opts)

	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "1", results[0].ID)
	assert.Equal(t, "search result", results[0].Content)
}

func TestMemoryStoreAdapter_Get(t *testing.T) {
	adapter, mem0 := newTestAdapter()
	ctx := context.Background()

	mem0.entries["test-1"] = &types.MemoryEntry{
		ID:      "test-1",
		Content: "stored content",
	}

	mem, err := adapter.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", mem.ID)
	assert.Equal(t, "stored content", mem.Content)
}

func TestMemoryStoreAdapter_Update(t *testing.T) {
	adapter, mem0 := newTestAdapter()
	ctx := context.Background()

	mem0.entries["test-1"] = &types.MemoryEntry{
		ID:      "test-1",
		Content: "original",
		Source:  types.SourceMem0,
	}

	mem := &modstore.Memory{
		ID:      "test-1",
		Content: "updated",
		Metadata: map[string]any{
			"helix_source": "mem0",
		},
	}

	err := adapter.Update(ctx, mem)
	assert.NoError(t, err)
}

func TestMemoryStoreAdapter_Delete(t *testing.T) {
	adapter, mem0 := newTestAdapter()
	ctx := context.Background()

	mem0.entries["test-1"] = &types.MemoryEntry{ID: "test-1"}

	err := adapter.Delete(ctx, "test-1")
	assert.NoError(t, err)
	assert.Empty(t, mem0.entries)
}

func TestMemoryStoreAdapter_List(t *testing.T) {
	adapter, mem0 := newTestAdapter()
	ctx := context.Background()

	mem0.entries["1"] = &types.MemoryEntry{
		ID: "1", Content: "list item", Source: types.SourceMem0,
		Type: types.MemoryTypeFact, CreatedAt: time.Now(),
	}

	opts := &modstore.ListOptions{Limit: 100}
	mems, err := adapter.List(ctx, modstore.ScopeUser, opts)

	require.NoError(t, err)
	assert.Len(t, mems, 1)
}

func TestModuleMemoryToEntry(t *testing.T) {
	now := time.Now()
	m := &modstore.Memory{
		ID:      "test-1",
		Content: "test content",
		Metadata: map[string]any{
			"key": "value",
		},
		Scope:     modstore.ScopeUser,
		CreatedAt: now,
		UpdatedAt: now,
		Score:     0.85,
		Embedding: []float32{0.1, 0.2},
	}

	entry := moduleMemoryToEntry(m)
	assert.Equal(t, "test-1", entry.ID)
	assert.Equal(t, "test content", entry.Content)
	assert.Equal(t, types.MemoryTypeFact, entry.Type)
	assert.Equal(t, types.SourceFusion, entry.Source)
	assert.InDelta(t, 0.85, entry.Relevance, 0.01)
	assert.Equal(t, "value", entry.Metadata["key"])
	assert.Equal(t, "user", entry.Metadata["scope"])
	assert.Len(t, entry.Embedding, 2)
}

func TestModuleMemoryToEntry_Nil(t *testing.T) {
	entry := moduleMemoryToEntry(nil)
	assert.Nil(t, entry)
}

func TestEntryToModuleMemory(t *testing.T) {
	now := time.Now()
	entry := &types.MemoryEntry{
		ID:         "test-1",
		Content:    "test content",
		Type:       types.MemoryTypeFact,
		Source:     types.SourceMem0,
		Confidence: 0.9,
		Relevance:  0.85,
		Metadata:   map[string]interface{}{"key": "value"},
		Embedding:  []float32{0.1, 0.2},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	mem := entryToModuleMemory(entry)
	assert.Equal(t, "test-1", mem.ID)
	assert.Equal(t, "test content", mem.Content)
	assert.Equal(t, modstore.ScopeUser, mem.Scope)
	assert.InDelta(t, 0.85, mem.Score, 0.01)
	assert.Equal(t, "value", mem.Metadata["key"])
	assert.Equal(t, "mem0", mem.Metadata["helix_source"])
	assert.Equal(t, "fact", mem.Metadata["helix_type"])
	assert.Len(t, mem.Embedding, 2)
}

func TestEntryToModuleMemory_Nil(t *testing.T) {
	mem := entryToModuleMemory(nil)
	assert.Nil(t, mem)
}

func TestEntryToModuleMemory_ZeroTime(t *testing.T) {
	entry := &types.MemoryEntry{
		ID:      "test-1",
		Content: "content",
	}

	mem := entryToModuleMemory(entry)
	assert.False(t, mem.CreatedAt.IsZero())
	assert.False(t, mem.UpdatedAt.IsZero())
}

func TestMemoryStoreAdapter_ImplementsInterface(t *testing.T) {
	// Compile-time check
	var _ modstore.MemoryStore = (*MemoryStoreAdapter)(nil)
}
