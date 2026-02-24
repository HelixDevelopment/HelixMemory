// Package e2e provides end-to-end tests for the HelixMemory unified cognitive
// memory engine, validating complete workflows from add through search to
// delete across the full provider stack.
package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/provider"
	"digital.vasic.helixmemory/pkg/types"

	modstore "digital.vasic.memory/pkg/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is a thread-safe mock provider for E2E testing.
type testProvider struct {
	mu      sync.RWMutex
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
	p.mu.Lock()
	defer p.mu.Unlock()
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
		return fmt.Errorf("unhealthy")
	}
	return nil
}

// coreMemoryProvider extends testProvider with Letta CoreMemoryProvider.
type coreMemoryProvider struct {
	*testProvider
	mu     sync.RWMutex
	blocks map[string][]*types.CoreMemoryBlock
}

func newCoreMemoryProvider() *coreMemoryProvider {
	return &coreMemoryProvider{
		testProvider: newTestProvider(types.SourceLetta),
		blocks:       make(map[string][]*types.CoreMemoryBlock),
	}
}

func (p *coreMemoryProvider) GetCoreMemory(
	_ context.Context, agentID string,
) ([]*types.CoreMemoryBlock, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if blocks, ok := p.blocks[agentID]; ok {
		return blocks, nil
	}
	return []*types.CoreMemoryBlock{}, nil
}

func (p *coreMemoryProvider) UpdateCoreMemory(
	_ context.Context, agentID string, block *types.CoreMemoryBlock,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	blocks := p.blocks[agentID]
	for i, b := range blocks {
		if b.Label == block.Label {
			blocks[i] = block
			p.blocks[agentID] = blocks
			return nil
		}
	}
	p.blocks[agentID] = append(p.blocks[agentID], block)
	return nil
}

// temporalProvider extends testProvider with Graphiti TemporalProvider.
type temporalProvider struct {
	*testProvider
}

func newTemporalProvider() *temporalProvider {
	return &temporalProvider{
		testProvider: newTestProvider(types.SourceGraphiti),
	}
}

func (p *temporalProvider) SearchTemporal(
	_ context.Context, _ string, at time.Time,
) ([]*types.MemoryEntry, error) {
	p.testProvider.mu.RLock()
	defer p.testProvider.mu.RUnlock()
	var results []*types.MemoryEntry
	for _, e := range p.testProvider.entries {
		if e.CreatedAt.Before(at) || e.CreatedAt.Equal(at) {
			results = append(results, e)
		}
	}
	return results, nil
}

func (p *temporalProvider) GetTimeline(
	_ context.Context, _ string, start, end time.Time,
) ([]*types.MemoryEntry, error) {
	p.testProvider.mu.RLock()
	defer p.testProvider.mu.RUnlock()
	var results []*types.MemoryEntry
	for _, e := range p.testProvider.entries {
		if (e.CreatedAt.After(start) || e.CreatedAt.Equal(start)) &&
			(e.CreatedAt.Before(end) || e.CreatedAt.Equal(end)) {
			results = append(results, e)
		}
	}
	return results, nil
}

func (p *temporalProvider) InvalidateAt(
	_ context.Context, id string, _ time.Time,
) error {
	p.testProvider.mu.Lock()
	defer p.testProvider.mu.Unlock()
	delete(p.testProvider.entries, id)
	return nil
}

// newE2ESetup creates a full multi-provider setup.
func newE2ESetup() (
	*provider.UnifiedMemoryProvider,
	*testProvider, *testProvider,
	*coreMemoryProvider, *temporalProvider,
) {
	cfg := config.DefaultConfig()
	u := provider.New(cfg)

	mem0 := newTestProvider(types.SourceMem0)
	cognee := newTestProvider(types.SourceCognee)
	letta := newCoreMemoryProvider()
	graphiti := newTemporalProvider()

	u.RegisterProvider(mem0)
	u.RegisterProvider(cognee)
	u.RegisterProvider(letta)
	u.RegisterProvider(graphiti)

	return u, mem0, cognee, letta, graphiti
}

func TestE2E_AddSearchGetUpdateDelete(t *testing.T) {
	u, _, _, _, _ := newE2ESetup()
	ctx := context.Background()

	// Step 1: Add a memory entry
	entry := &types.MemoryEntry{
		ID:      "e2e-crud-1",
		Content: "Go is a statically typed language",
		Type:    types.MemoryTypeFact,
		Source:  types.SourceMem0,
	}
	err := u.Add(ctx, entry)
	require.NoError(t, err, "add should succeed")

	// Step 2: Search for it
	req := types.DefaultSearchRequest("Go language")
	result, err := u.Search(ctx, req)
	require.NoError(t, err, "search should succeed")
	require.NotNil(t, result)
	// The entry should appear in results from the mem0 provider
	found := false
	for _, e := range result.Entries {
		if e.ID == "e2e-crud-1" {
			found = true
			break
		}
	}
	assert.True(t, found, "added entry should appear in search results")

	// Step 3: Get by ID
	got, err := u.Get(ctx, "e2e-crud-1")
	require.NoError(t, err, "get should succeed")
	assert.Equal(t, "Go is a statically typed language", got.Content)

	// Step 4: Update
	got.Content = "Go is a compiled, statically typed language"
	got.Source = types.SourceMem0
	err = u.Update(ctx, got)
	require.NoError(t, err, "update should succeed")

	// Verify update
	got2, err := u.Get(ctx, "e2e-crud-1")
	require.NoError(t, err)
	assert.Equal(t, "Go is a compiled, statically typed language", got2.Content)

	// Step 5: Delete
	err = u.Delete(ctx, "e2e-crud-1")
	require.NoError(t, err, "delete should succeed")

	// Verify deletion
	_, err = u.Get(ctx, "e2e-crud-1")
	assert.Error(t, err, "entry should not be found after deletion")
}

func TestE2E_MemoryStoreAdapter(t *testing.T) {
	u, _, _, _, _ := newE2ESetup()
	ctx := context.Background()

	adapter := provider.NewMemoryStoreAdapter(u)

	// Verify interface compliance
	var _ modstore.MemoryStore = adapter

	// Add via adapter
	mem := &modstore.Memory{
		ID:      "adapter-1",
		Content: "adapter test content",
		Scope:   modstore.ScopeUser,
		Metadata: map[string]any{
			"key": "value",
		},
	}
	err := adapter.Add(ctx, mem)
	require.NoError(t, err, "adapter add should succeed")

	// Get via adapter
	got, err := adapter.Get(ctx, "adapter-1")
	require.NoError(t, err, "adapter get should succeed")
	assert.Equal(t, "adapter-1", got.ID)
	assert.Equal(t, "adapter test content", got.Content)

	// Search via adapter
	searchResults, err := adapter.Search(ctx, "adapter test",
		&modstore.SearchOptions{TopK: 10})
	require.NoError(t, err, "adapter search should succeed")
	assert.NotEmpty(t, searchResults, "adapter search should find results")

	// Update via adapter
	mem.Content = "adapter updated content"
	err = adapter.Update(ctx, mem)
	require.NoError(t, err, "adapter update should succeed")

	// List via adapter
	listResults, err := adapter.List(ctx, modstore.ScopeUser,
		&modstore.ListOptions{Limit: 100})
	require.NoError(t, err, "adapter list should succeed")
	assert.NotEmpty(t, listResults)

	// Delete via adapter
	err = adapter.Delete(ctx, "adapter-1")
	require.NoError(t, err, "adapter delete should succeed")

	_, err = adapter.Get(ctx, "adapter-1")
	assert.Error(t, err, "deleted entry should not be found via adapter")
}

func TestE2E_SearchResultFusion(t *testing.T) {
	u, mem0, cognee, _, _ := newE2ESetup()
	ctx := context.Background()

	now := time.Now()

	// Add entries to multiple providers
	mem0.entries["fusion-1"] = &types.MemoryEntry{
		ID: "fusion-1", Content: "user prefers dark mode",
		Source: types.SourceMem0, Type: types.MemoryTypeFact,
		Relevance: 0.9, CreatedAt: now,
	}
	mem0.entries["fusion-2"] = &types.MemoryEntry{
		ID: "fusion-2", Content: "user favorite language is go",
		Source: types.SourceMem0, Type: types.MemoryTypeFact,
		Relevance: 0.85, CreatedAt: now.Add(-time.Hour),
	}
	cognee.entries["fusion-3"] = &types.MemoryEntry{
		ID: "fusion-3", Content: "go relates to system programming",
		Source: types.SourceCognee, Type: types.MemoryTypeGraph,
		Relevance: 0.8, CreatedAt: now.Add(-2 * time.Hour),
	}

	req := &types.SearchRequest{
		Query: "user preferences",
		TopK:  10,
	}
	result, err := u.Search(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Entries, 3, "should fuse results from both providers")
	// Results should be ranked by the fusion formula
	for i := 0; i < len(result.Entries)-1; i++ {
		assert.GreaterOrEqual(t,
			result.Entries[i].Relevance,
			result.Entries[i+1].Relevance,
			"entries should be ranked by fused relevance score")
	}
}

func TestE2E_CoreMemoryBlocks(t *testing.T) {
	u, _, _, letta, _ := newE2ESetup()
	ctx := context.Background()

	// Seed core memory blocks
	letta.blocks["agent-1"] = []*types.CoreMemoryBlock{
		{Label: "persona", Value: "I am a helpful coding assistant", Limit: 2000},
		{Label: "human", Value: "The user is a Go developer", Limit: 2000},
	}

	// Get core memory
	blocks, err := u.GetCoreMemory(ctx, "agent-1")
	require.NoError(t, err, "should retrieve core memory blocks")
	assert.Len(t, blocks, 2)

	// Verify block contents
	blockMap := make(map[string]string)
	for _, b := range blocks {
		blockMap[b.Label] = b.Value
	}
	assert.Equal(t, "I am a helpful coding assistant", blockMap["persona"])
	assert.Equal(t, "The user is a Go developer", blockMap["human"])

	// Update core memory
	err = u.UpdateCoreMemory(ctx, "agent-1", &types.CoreMemoryBlock{
		Label: "persona",
		Value: "I am a helpful Go coding assistant",
		Limit: 2000,
	})
	require.NoError(t, err, "should update core memory block")

	// Verify update
	blocks, err = u.GetCoreMemory(ctx, "agent-1")
	require.NoError(t, err)
	for _, b := range blocks {
		if b.Label == "persona" {
			assert.Equal(t,
				"I am a helpful Go coding assistant", b.Value)
		}
	}
}

func TestE2E_TemporalQueries(t *testing.T) {
	u, _, _, _, graphiti := newE2ESetup()
	ctx := context.Background()

	now := time.Now()

	// Seed temporal entries
	graphiti.testProvider.entries["temp-1"] = &types.MemoryEntry{
		ID: "temp-1", Content: "API v1 was deployed",
		Source: types.SourceGraphiti, Type: types.MemoryTypeTemporal,
		CreatedAt: now.Add(-48 * time.Hour),
	}
	graphiti.testProvider.entries["temp-2"] = &types.MemoryEntry{
		ID: "temp-2", Content: "API v2 was deployed",
		Source: types.SourceGraphiti, Type: types.MemoryTypeTemporal,
		CreatedAt: now.Add(-24 * time.Hour),
	}
	graphiti.testProvider.entries["temp-3"] = &types.MemoryEntry{
		ID: "temp-3", Content: "API v3 was deployed",
		Source: types.SourceGraphiti, Type: types.MemoryTypeTemporal,
		CreatedAt: now,
	}

	// Search temporal -- at 25 hours ago should include temp-1 only
	results, err := u.SearchTemporal(ctx, "API deployment",
		now.Add(-25*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 1,
		"should only return entries before the temporal point")
	assert.Equal(t, "temp-1", results[0].ID)

	// Search temporal -- at now should include all three
	results, err = u.SearchTemporal(ctx, "API deployment", now)
	require.NoError(t, err)
	assert.Len(t, results, 3,
		"should return all entries up to the given time")
}

func TestE2E_MultiProviderHealthCheck(t *testing.T) {
	u, mem0, cognee, letta, graphiti := newE2ESetup()
	ctx := context.Background()

	// All healthy
	err := u.Health(ctx)
	assert.NoError(t, err)

	// Make some unhealthy
	mem0.mu.Lock()
	mem0.healthy = false
	mem0.mu.Unlock()

	graphiti.testProvider.mu.Lock()
	graphiti.testProvider.healthy = false
	graphiti.testProvider.mu.Unlock()

	// Still healthy (cognee and letta are up)
	err = u.Health(ctx)
	assert.NoError(t, err,
		"should be healthy with at least one provider up")

	// Check detailed
	detailed := u.HealthDetailed(ctx)
	assert.Error(t, detailed[types.SourceMem0])
	assert.NoError(t, detailed[types.SourceCognee])
	assert.NoError(t, detailed[types.SourceLetta])
	assert.Error(t, detailed[types.SourceGraphiti])

	// All unhealthy
	cognee.mu.Lock()
	cognee.healthy = false
	cognee.mu.Unlock()
	letta.testProvider.mu.Lock()
	letta.testProvider.healthy = false
	letta.testProvider.mu.Unlock()

	err = u.Health(ctx)
	assert.Error(t, err, "should be unhealthy when all providers are down")
}
