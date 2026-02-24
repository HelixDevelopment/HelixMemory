package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is a mock provider for unit testing.
type testProvider struct {
	name    types.MemorySource
	entries map[string]*types.MemoryEntry
	healthy bool
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
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
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

func (p *testProvider) GetHistory(_ context.Context, _ string, limit int) ([]*types.MemoryEntry, error) {
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

func newTestUnified() (*UnifiedMemoryProvider, *testProvider, *testProvider) {
	cfg := config.DefaultConfig()
	u := New(cfg)

	mem0 := newTestProvider(types.SourceMem0)
	cognee := newTestProvider(types.SourceCognee)

	u.RegisterProvider(mem0)
	u.RegisterProvider(cognee)

	return u, mem0, cognee
}

func TestUnifiedMemoryProvider_RegisterProvider(t *testing.T) {
	u, _, _ := newTestUnified()

	sources := u.AvailableProviders()
	assert.Len(t, sources, 2)
}

func TestUnifiedMemoryProvider_Name(t *testing.T) {
	u, _, _ := newTestUnified()
	assert.Equal(t, types.SourceFusion, u.Name())
}

func TestUnifiedMemoryProvider_Add(t *testing.T) {
	u, mem0, _ := newTestUnified()
	ctx := context.Background()

	entry := &types.MemoryEntry{
		ID:      "test-1",
		Content: "Go is a great language",
		Type:    types.MemoryTypeFact,
		Source:  types.SourceMem0,
	}

	err := u.Add(ctx, entry)
	assert.NoError(t, err)

	// Should be routed to mem0 (fact type)
	assert.Len(t, mem0.entries, 1)
}

func TestUnifiedMemoryProvider_Search(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Add entries to both providers
	mem0.entries["1"] = &types.MemoryEntry{
		ID: "1", Content: "fact from mem0", Source: types.SourceMem0,
		Type: types.MemoryTypeFact, CreatedAt: time.Now(),
	}
	cognee.entries["2"] = &types.MemoryEntry{
		ID: "2", Content: "graph from cognee", Source: types.SourceCognee,
		Type: types.MemoryTypeGraph, CreatedAt: time.Now(),
	}

	req := types.DefaultSearchRequest("test")
	result, err := u.Search(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Entries, 2)
}

func TestUnifiedMemoryProvider_Get(t *testing.T) {
	u, mem0, _ := newTestUnified()
	ctx := context.Background()

	mem0.entries["test-1"] = &types.MemoryEntry{
		ID:      "test-1",
		Content: "test content",
	}

	entry, err := u.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", entry.ID)
	assert.Equal(t, "test content", entry.Content)
}

func TestUnifiedMemoryProvider_Get_NotFound(t *testing.T) {
	u, _, _ := newTestUnified()
	ctx := context.Background()

	_, err := u.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestUnifiedMemoryProvider_Update(t *testing.T) {
	u, mem0, _ := newTestUnified()
	ctx := context.Background()

	mem0.entries["test-1"] = &types.MemoryEntry{
		ID:      "test-1",
		Content: "original",
		Source:  types.SourceMem0,
	}

	err := u.Update(ctx, &types.MemoryEntry{
		ID:      "test-1",
		Content: "updated",
		Source:  types.SourceMem0,
	})
	assert.NoError(t, err)
	assert.Equal(t, "updated", mem0.entries["test-1"].Content)
}

func TestUnifiedMemoryProvider_Delete(t *testing.T) {
	u, mem0, _ := newTestUnified()
	ctx := context.Background()

	mem0.entries["test-1"] = &types.MemoryEntry{ID: "test-1"}

	err := u.Delete(ctx, "test-1")
	assert.NoError(t, err)
	assert.Empty(t, mem0.entries)
}

func TestUnifiedMemoryProvider_Health(t *testing.T) {
	u, _, _ := newTestUnified()
	ctx := context.Background()

	err := u.Health(ctx)
	assert.NoError(t, err)
}

func TestUnifiedMemoryProvider_Health_AllUnhealthy(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	mem0.healthy = false
	cognee.healthy = false

	err := u.Health(ctx)
	assert.Error(t, err)
}

func TestUnifiedMemoryProvider_HealthDetailed(t *testing.T) {
	u, mem0, _ := newTestUnified()
	ctx := context.Background()

	mem0.healthy = false

	health := u.HealthDetailed(ctx)
	assert.Error(t, health[types.SourceMem0])
	assert.NoError(t, health[types.SourceCognee])
}

func TestUnifiedMemoryProvider_GracefulDegradation(t *testing.T) {
	cfg := config.DefaultConfig()
	u := New(cfg)

	healthy := newTestProvider(types.SourceMem0)
	healthy.entries["1"] = &types.MemoryEntry{
		ID: "1", Content: "available", Source: types.SourceMem0,
		Type: types.MemoryTypeFact, CreatedAt: time.Now(),
	}

	u.RegisterProvider(healthy)

	// Only one provider — search should still work
	ctx := context.Background()
	result, err := u.Search(ctx, types.DefaultSearchRequest("test"))

	require.NoError(t, err)
	assert.Len(t, result.Entries, 1)
}

func TestUnifiedMemoryProvider_NoProviders(t *testing.T) {
	cfg := config.DefaultConfig()
	u := New(cfg)
	ctx := context.Background()

	// Search with no providers
	result, err := u.Search(ctx, types.DefaultSearchRequest("test"))
	require.NoError(t, err)
	assert.Empty(t, result.Entries)

	// Health with no providers
	err = u.Health(ctx)
	assert.Error(t, err)
}

func TestUnifiedMemoryProvider_GetProvider(t *testing.T) {
	u, _, _ := newTestUnified()

	p, ok := u.GetProvider(types.SourceMem0)
	assert.True(t, ok)
	assert.NotNil(t, p)

	_, ok = u.GetProvider(types.SourceGraphiti)
	assert.False(t, ok)
}

func TestUnifiedMemoryProvider_GetHistory(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	mem0.entries["1"] = &types.MemoryEntry{
		ID: "1", Content: "mem0 history", CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	cognee.entries["2"] = &types.MemoryEntry{
		ID: "2", Content: "cognee history", CreatedAt: time.Now(),
	}

	entries, err := u.GetHistory(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	// Should be sorted by time, newest first
	assert.True(t, entries[0].CreatedAt.After(entries[1].CreatedAt) ||
		entries[0].CreatedAt.Equal(entries[1].CreatedAt))
}
