// Package integration provides integration tests for the HelixMemory
// unified cognitive memory engine, validating cross-component behavior
// with mock providers.
package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/provider"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is a thread-safe mock provider for integration testing.
type testProvider struct {
	mu      sync.RWMutex
	name    types.MemorySource
	entries map[string]*types.MemoryEntry
	healthy bool
	addErr  error
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.addErr != nil {
		return p.addErr
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Search(
	_ context.Context, req *types.SearchRequest,
) (*types.SearchResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.healthy {
		return nil, fmt.Errorf("provider %s unhealthy", p.name)
	}
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		// Return a copy to avoid data races when fusion engine mutates Relevance
		cp := *e
		entries = append(entries, &cp)
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
	p.mu.RLock()
	defer p.mu.RUnlock()
	if e, ok := p.entries[id]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (p *testProvider) Update(_ context.Context, entry *types.MemoryEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.entries[entry.ID]; !ok {
		return fmt.Errorf("not found: %s", entry.ID)
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Delete(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, id)
	return nil
}

func (p *testProvider) GetHistory(
	_ context.Context, _ string, limit int,
) ([]*types.MemoryEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		entries = append(entries, e)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

func (p *testProvider) Health(_ context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.healthy {
		return fmt.Errorf("provider %s unhealthy", p.name)
	}
	return nil
}

func (p *testProvider) setHealthy(h bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthy = h
}

func (p *testProvider) setAddErr(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.addErr = err
}

func (p *testProvider) entryCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}

// newTestUnified creates a UnifiedMemoryProvider with two test providers.
func newTestUnified() (
	*provider.UnifiedMemoryProvider, *testProvider, *testProvider,
) {
	cfg := config.DefaultConfig()
	u := provider.New(cfg)

	mem0 := newTestProvider(types.SourceMem0)
	cognee := newTestProvider(types.SourceCognee)

	u.RegisterProvider(mem0)
	u.RegisterProvider(cognee)

	return u, mem0, cognee
}

func TestUnifiedProvider_RegisterAndSearch(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Seed data into both providers
	mem0.entries["m1"] = &types.MemoryEntry{
		ID: "m1", Content: "fact from mem0",
		Source: types.SourceMem0, Type: types.MemoryTypeFact,
		Relevance: 0.9, CreatedAt: time.Now(),
	}
	cognee.entries["c1"] = &types.MemoryEntry{
		ID: "c1", Content: "graph from cognee",
		Source: types.SourceCognee, Type: types.MemoryTypeGraph,
		Relevance: 0.85, CreatedAt: time.Now(),
	}

	req := types.DefaultSearchRequest("test query")
	result, err := u.Search(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Entries, 2, "should fuse results from both providers")
	assert.True(t, result.Total > 0)

	// Verify both sources are represented
	ids := make(map[string]bool)
	for _, e := range result.Entries {
		ids[e.ID] = true
	}
	assert.True(t, ids["m1"] || ids["c1"],
		"at least one original ID should be present")
}

func TestUnifiedProvider_AddWithRouting(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Fact type routes to mem0
	factEntry := &types.MemoryEntry{
		ID: "fact-1", Content: "Go is compiled",
		Type: types.MemoryTypeFact, Source: types.SourceMem0,
	}
	err := u.Add(ctx, factEntry)
	require.NoError(t, err)
	assert.Equal(t, 1, mem0.entryCount(),
		"fact should be routed to mem0")

	// Graph type routes to cognee
	graphEntry := &types.MemoryEntry{
		ID: "graph-1", Content: "auth depends on database",
		Type: types.MemoryTypeGraph, Source: types.SourceCognee,
	}
	err = u.Add(ctx, graphEntry)
	require.NoError(t, err)
	assert.Equal(t, 1, cognee.entryCount(),
		"graph should be routed to cognee")
}

func TestUnifiedProvider_GracefulDegradation(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Seed data into both
	mem0.entries["m1"] = &types.MemoryEntry{
		ID: "m1", Content: "fact from mem0",
		Source: types.SourceMem0, Type: types.MemoryTypeFact,
		Relevance: 0.9, CreatedAt: time.Now(),
	}
	cognee.entries["c1"] = &types.MemoryEntry{
		ID: "c1", Content: "graph from cognee",
		Source: types.SourceCognee, Type: types.MemoryTypeGraph,
		Relevance: 0.85, CreatedAt: time.Now(),
	}

	// Mark cognee as unhealthy
	cognee.setHealthy(false)

	req := types.DefaultSearchRequest("test")
	result, err := u.Search(ctx, req)

	require.NoError(t, err, "search should succeed even if one provider fails")
	require.NotNil(t, result)
	// Only mem0 results should come back since cognee is unhealthy
	assert.GreaterOrEqual(t, len(result.Entries), 1,
		"should still return results from healthy provider")
}

func TestUnifiedProvider_HealthAggregation(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Both healthy
	err := u.Health(ctx)
	assert.NoError(t, err, "should be healthy when all providers are healthy")

	// One unhealthy -- partial health is still OK
	mem0.setHealthy(false)
	err = u.Health(ctx)
	assert.NoError(t, err,
		"should still be healthy if at least one provider is up")

	// Verify detailed health
	detailed := u.HealthDetailed(ctx)
	assert.Error(t, detailed[types.SourceMem0],
		"mem0 should report unhealthy")
	assert.NoError(t, detailed[types.SourceCognee],
		"cognee should report healthy")

	// All unhealthy
	cognee.setHealthy(false)
	err = u.Health(ctx)
	assert.Error(t, err, "should be unhealthy when all providers are down")
}

func TestUnifiedProvider_ConcurrentSearch(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Seed data
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("mem0-%d", i)
		mem0.entries[id] = &types.MemoryEntry{
			ID: id, Content: fmt.Sprintf("mem0 entry %d", i),
			Source: types.SourceMem0, Type: types.MemoryTypeFact,
			Relevance: float64(i) / 20.0, CreatedAt: time.Now(),
		}
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("cognee-%d", i)
		cognee.entries[id] = &types.MemoryEntry{
			ID: id, Content: fmt.Sprintf("cognee entry %d", i),
			Source: types.SourceCognee, Type: types.MemoryTypeGraph,
			Relevance: float64(i) / 20.0, CreatedAt: time.Now(),
		}
	}

	const goroutines = 50
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := types.DefaultSearchRequest("concurrent test")
			result, err := u.Search(ctx, req)
			if err != nil || result == nil {
				errCount.Add(1)
				return
			}
			if len(result.Entries) == 0 {
				errCount.Add(1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load(),
		"all concurrent searches should succeed")
}

func TestUnifiedProvider_GetAcrossProviders(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Entry only in cognee
	cognee.entries["cognee-only"] = &types.MemoryEntry{
		ID: "cognee-only", Content: "only in cognee",
		Source: types.SourceCognee,
	}

	// Entry only in mem0
	mem0.entries["mem0-only"] = &types.MemoryEntry{
		ID: "mem0-only", Content: "only in mem0",
		Source: types.SourceMem0,
	}

	// Get should find entry in cognee
	entry, err := u.Get(ctx, "cognee-only")
	require.NoError(t, err)
	assert.Equal(t, "cognee-only", entry.ID)
	assert.Equal(t, "only in cognee", entry.Content)

	// Get should find entry in mem0
	entry, err = u.Get(ctx, "mem0-only")
	require.NoError(t, err)
	assert.Equal(t, "mem0-only", entry.ID)
	assert.Equal(t, "only in mem0", entry.Content)

	// Get non-existent entry should fail
	_, err = u.Get(ctx, "nonexistent")
	assert.Error(t, err, "should return error for nonexistent entries")
}

func TestUnifiedProvider_DeleteAcrossProviders(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Entries in both providers
	mem0.entries["shared-1"] = &types.MemoryEntry{
		ID: "shared-1", Content: "shared entry",
	}
	cognee.entries["shared-1"] = &types.MemoryEntry{
		ID: "shared-1", Content: "shared entry copy",
	}

	err := u.Delete(ctx, "shared-1")
	require.NoError(t, err)

	// Both providers should have the entry removed
	assert.Equal(t, 0, mem0.entryCount(),
		"mem0 should have entry deleted")
	assert.Equal(t, 0, cognee.entryCount(),
		"cognee should have entry deleted")

	// Verify it is actually gone
	_, err = u.Get(ctx, "shared-1")
	assert.Error(t, err, "deleted entry should not be found")
}

func TestUnifiedProvider_AddFallbackOnPrimaryFailure(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Make mem0 fail on add
	mem0.setAddErr(fmt.Errorf("mem0 write failure"))

	// Fact would normally route to mem0, but should fallback to cognee
	entry := &types.MemoryEntry{
		ID: "fallback-1", Content: "should fallback",
		Type: types.MemoryTypeFact,
	}
	err := u.Add(ctx, entry)
	require.NoError(t, err, "should succeed via fallback provider")
	assert.Equal(t, 1, cognee.entryCount(),
		"entry should be in fallback provider (cognee)")
}

func TestUnifiedProvider_UpdateRouting(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	// Entry exists in mem0
	mem0.entries["update-1"] = &types.MemoryEntry{
		ID: "update-1", Content: "original content",
		Source: types.SourceMem0,
	}

	// Update with source set should go directly to the source provider
	updated := &types.MemoryEntry{
		ID: "update-1", Content: "updated content",
		Source: types.SourceMem0,
	}
	err := u.Update(ctx, updated)
	require.NoError(t, err)

	entry, err := u.Get(ctx, "update-1")
	require.NoError(t, err)
	assert.Equal(t, "updated content", entry.Content)

	// Update without source should try all providers
	cognee.entries["update-2"] = &types.MemoryEntry{
		ID: "update-2", Content: "cognee original",
		Source: types.SourceCognee,
	}
	err = u.Update(ctx, &types.MemoryEntry{
		ID: "update-2", Content: "updated via broadcast",
	})
	require.NoError(t, err)
}

func TestUnifiedProvider_GetHistory(t *testing.T) {
	u, mem0, cognee := newTestUnified()
	ctx := context.Background()

	now := time.Now()
	mem0.entries["h1"] = &types.MemoryEntry{
		ID: "h1", Content: "mem0 history",
		CreatedAt: now.Add(-2 * time.Hour),
	}
	cognee.entries["h2"] = &types.MemoryEntry{
		ID: "h2", Content: "cognee history",
		CreatedAt: now.Add(-1 * time.Hour),
	}
	mem0.entries["h3"] = &types.MemoryEntry{
		ID: "h3", Content: "mem0 recent",
		CreatedAt: now,
	}

	entries, err := u.GetHistory(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.Len(t, entries, 3, "should merge history from all providers")
	// Should be sorted newest first
	for i := 0; i < len(entries)-1; i++ {
		assert.True(t,
			entries[i].CreatedAt.After(entries[i+1].CreatedAt) ||
				entries[i].CreatedAt.Equal(entries[i+1].CreatedAt),
			"history should be sorted newest first")
	}
}
