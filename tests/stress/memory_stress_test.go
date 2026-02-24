// Package stress provides stress tests for the HelixMemory system, validating
// concurrent access patterns, high-throughput operation handling, and provider
// recovery scenarios.
package stress

import (
	"context"
	"fmt"
	"math/rand/v2"
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

// testProvider is a thread-safe mock provider for stress testing.
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
	if !p.healthy {
		return fmt.Errorf("provider %s unhealthy", p.name)
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

func (p *testProvider) entryCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}

func newStressUnified() (
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

func TestStress_ConcurrentAdds(t *testing.T) {
	u, mem0, cognee := newStressUnified()
	ctx := context.Background()

	const goroutines = 100
	var wg sync.WaitGroup
	var errCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			entry := &types.MemoryEntry{
				ID:      fmt.Sprintf("stress-add-%d", idx),
				Content: fmt.Sprintf("stress test content %d", idx),
				Type:    types.MemoryTypeFact,
				Source:  types.SourceMem0,
			}
			if err := u.Add(ctx, entry); err != nil {
				errCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load(),
		"all concurrent adds should succeed without errors")

	totalEntries := mem0.entryCount() + cognee.entryCount()
	assert.Equal(t, goroutines, totalEntries,
		"all entries should be stored across providers")
}

func TestStress_ConcurrentSearches(t *testing.T) {
	u, mem0, cognee := newStressUnified()
	ctx := context.Background()

	// Pre-seed data
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("seed-mem0-%d", i)
		mem0.entries[id] = &types.MemoryEntry{
			ID: id, Content: fmt.Sprintf("mem0 seed entry %d", i),
			Source: types.SourceMem0, Type: types.MemoryTypeFact,
			Relevance: float64(i) / 50.0, CreatedAt: time.Now(),
		}
	}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("seed-cognee-%d", i)
		cognee.entries[id] = &types.MemoryEntry{
			ID: id, Content: fmt.Sprintf("cognee seed entry %d", i),
			Source: types.SourceCognee, Type: types.MemoryTypeGraph,
			Relevance: float64(i) / 50.0, CreatedAt: time.Now(),
		}
	}

	const goroutines = 100
	var wg sync.WaitGroup
	var errCount atomic.Int64
	var emptyCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := types.DefaultSearchRequest("seed entry")
			result, err := u.Search(ctx, req)
			if err != nil {
				errCount.Add(1)
				return
			}
			if result == nil || len(result.Entries) == 0 {
				emptyCount.Add(1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load(),
		"no search errors under concurrent load")
	assert.Equal(t, int64(0), emptyCount.Load(),
		"no empty results when data exists")
}

func TestStress_MixedOperations(t *testing.T) {
	u, _, _ := newStressUnified()
	ctx := context.Background()

	const goroutines = 100
	var wg sync.WaitGroup
	var errCount atomic.Int64

	// Pre-seed some entries
	for i := 0; i < 20; i++ {
		entry := &types.MemoryEntry{
			ID:        fmt.Sprintf("mixed-%d", i),
			Content:   fmt.Sprintf("mixed content %d", i),
			Type:      types.MemoryTypeFact,
			Source:    types.SourceMem0,
			CreatedAt: time.Now(),
		}
		require.NoError(t, u.Add(ctx, entry))
	}

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()

			op := idx % 5
			switch op {
			case 0: // Add
				entry := &types.MemoryEntry{
					ID:      fmt.Sprintf("mixed-new-%d", idx),
					Content: fmt.Sprintf("new content %d", idx),
					Type:    types.MemoryTypeFact,
					Source:  types.SourceMem0,
				}
				if err := u.Add(ctx, entry); err != nil {
					errCount.Add(1)
				}
			case 1: // Search
				req := types.DefaultSearchRequest("content")
				if _, err := u.Search(ctx, req); err != nil {
					errCount.Add(1)
				}
			case 2: // Get (may fail for non-existent, that is OK)
				targetID := fmt.Sprintf("mixed-%d", rand.IntN(20))
				_, _ = u.Get(ctx, targetID)
			case 3: // Update (may fail for non-existent)
				targetID := fmt.Sprintf("mixed-%d", rand.IntN(20))
				entry := &types.MemoryEntry{
					ID:      targetID,
					Content: fmt.Sprintf("updated at %d", idx),
					Source:  types.SourceMem0,
				}
				_ = u.Update(ctx, entry)
			case 4: // Delete (may fail for non-existent)
				targetID := fmt.Sprintf("mixed-%d", rand.IntN(20))
				_ = u.Delete(ctx, targetID)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(0), errCount.Load(),
		"add and search operations should never fail under mixed load")

	// The system should still be operational after the stress
	err := u.Health(ctx)
	assert.NoError(t, err, "system should remain healthy after stress test")
}

func TestStress_ProviderRecovery(t *testing.T) {
	u, mem0, cognee := newStressUnified()
	ctx := context.Background()

	// Pre-seed cognee
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("recovery-%d", i)
		cognee.entries[id] = &types.MemoryEntry{
			ID: id, Content: fmt.Sprintf("cognee recovery entry %d", i),
			Source: types.SourceCognee, Type: types.MemoryTypeGraph,
			Relevance: 0.8, CreatedAt: time.Now(),
		}
	}

	// Use high TopK to avoid result capping during the test
	req := &types.SearchRequest{Query: "recovery", TopK: 100}

	// Phase 1: Both providers healthy, everything works
	result, err := u.Search(ctx, req)
	require.NoError(t, err)
	initialCount := len(result.Entries)
	assert.Greater(t, initialCount, 0, "should have results initially")

	// Phase 2: Mem0 goes down mid-operation
	mem0.setHealthy(false)

	// Search should still work via cognee
	result, err = u.Search(ctx, req)
	require.NoError(t, err, "should degrade gracefully when mem0 is down")
	degradedCount := len(result.Entries)
	assert.Greater(t, degradedCount, 0,
		"cognee results should still be available")

	// Phase 3: Run concurrent operations during failure
	const goroutines = 50
	var wg sync.WaitGroup
	var searchErrs atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			searchReq := &types.SearchRequest{Query: "recovery", TopK: 100}
			_, sErr := u.Search(ctx, searchReq)
			if sErr != nil {
				searchErrs.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(0), searchErrs.Load(),
		"searches should not fail even with one provider down")

	// Phase 4: Mem0 recovers
	mem0.setHealthy(true)
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("post-recovery-%d", i)
		mem0.entries[id] = &types.MemoryEntry{
			ID: id, Content: fmt.Sprintf("mem0 post recovery %d", i),
			Source: types.SourceMem0, Type: types.MemoryTypeFact,
			Relevance: 0.9, CreatedAt: time.Now(),
		}
	}

	// Search should now include results from both providers
	result, err = u.Search(ctx, req)
	require.NoError(t, err)
	assert.Greater(t, len(result.Entries), degradedCount,
		"should have more results after recovery with new mem0 entries")
}
