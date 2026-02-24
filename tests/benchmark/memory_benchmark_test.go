// Package benchmark provides benchmark tests for the HelixMemory system,
// measuring performance of search, fusion, routing, and adapter operations.
package benchmark

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/fusion"
	"digital.vasic.helixmemory/pkg/provider"
	"digital.vasic.helixmemory/pkg/routing"
	"digital.vasic.helixmemory/pkg/types"

	modstore "digital.vasic.memory/pkg/store"
)

// testProvider is a thread-safe mock provider for benchmarking.
type testProvider struct {
	mu      sync.RWMutex
	name    types.MemorySource
	entries map[string]*types.MemoryEntry
}

func newTestProvider(name types.MemorySource) *testProvider {
	return &testProvider{
		name:    name,
		entries: make(map[string]*types.MemoryEntry),
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
		Duration: 1 * time.Microsecond,
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
	return nil
}

func seedProvider(p *testProvider, count int, source types.MemorySource) {
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("%s-%d", source, i)
		p.entries[id] = &types.MemoryEntry{
			ID:        id,
			Content:   fmt.Sprintf("benchmark entry %d from %s provider", i, source),
			Source:    source,
			Type:      types.MemoryTypeFact,
			Relevance: float64(count-i) / float64(count),
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
	}
}

func newBenchUnified(
	seedCount int,
) (*provider.UnifiedMemoryProvider, *testProvider, *testProvider) {
	cfg := config.DefaultConfig()
	u := provider.New(cfg)

	mem0 := newTestProvider(types.SourceMem0)
	cognee := newTestProvider(types.SourceCognee)

	seedProvider(mem0, seedCount, types.SourceMem0)
	seedProvider(cognee, seedCount, types.SourceCognee)

	u.RegisterProvider(mem0)
	u.RegisterProvider(cognee)

	return u, mem0, cognee
}

func BenchmarkUnifiedProvider_Search(b *testing.B) {
	sizes := []int{10, 100, 500}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("entries_%d", size*2), func(b *testing.B) {
			u, _, _ := newBenchUnified(size)
			ctx := context.Background()
			req := types.DefaultSearchRequest("benchmark entry")

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = u.Search(ctx, req)
			}
		})
	}
}

func BenchmarkFusionEngine_Fuse(b *testing.B) {
	sizes := []int{10, 50, 200}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("entries_%d", size), func(b *testing.B) {
			e := fusion.NewEngine(config.DefaultConfig())
			req := types.DefaultSearchRequest("benchmark")

			entries := make([]*types.MemoryEntry, size)
			for i := 0; i < size; i++ {
				entries[i] = &types.MemoryEntry{
					ID:        fmt.Sprintf("fuse-%d", i),
					Content:   fmt.Sprintf("unique benchmark content number %d", i),
					Source:    types.SourceMem0,
					Type:      types.MemoryTypeFact,
					Relevance: float64(size-i) / float64(size),
					CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute),
				}
			}

			results := []*types.SearchResult{
				{
					Entries: entries[:size/2],
					Sources: []types.MemorySource{types.SourceMem0},
				},
				{
					Entries: entries[size/2:],
					Sources: []types.MemorySource{types.SourceCognee},
				},
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = e.Fuse(results, req)
			}
		})
	}
}

func BenchmarkFusionEngine_Fuse_WithDedup(b *testing.B) {
	e := fusion.NewEngine(config.DefaultConfig())
	req := types.DefaultSearchRequest("benchmark")

	// Create entries where half are duplicates (identical content)
	entries1 := make([]*types.MemoryEntry, 50)
	entries2 := make([]*types.MemoryEntry, 50)
	for i := 0; i < 50; i++ {
		entries1[i] = &types.MemoryEntry{
			ID:        fmt.Sprintf("dedup-a-%d", i),
			Content:   fmt.Sprintf("shared content %d for dedup benchmark", i),
			Source:    types.SourceMem0,
			Type:      types.MemoryTypeFact,
			Relevance: 0.9,
			CreatedAt: time.Now(),
		}
		entries2[i] = &types.MemoryEntry{
			ID:        fmt.Sprintf("dedup-b-%d", i),
			Content:   fmt.Sprintf("shared content %d for dedup benchmark", i),
			Source:    types.SourceCognee,
			Type:      types.MemoryTypeGraph,
			Relevance: 0.85,
			CreatedAt: time.Now(),
		}
	}

	results := []*types.SearchResult{
		{Entries: entries1, Sources: []types.MemorySource{types.SourceMem0}},
		{Entries: entries2, Sources: []types.MemorySource{types.SourceCognee}},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = e.Fuse(results, req)
	}
}

func BenchmarkRouter_RouteWrite(b *testing.B) {
	r := routing.NewRouter()

	entries := []*types.MemoryEntry{
		{Content: "a simple fact about Go", Type: types.MemoryTypeFact},
		{Content: "auth depends on database", Type: types.MemoryTypeGraph},
		{Content: "i am a developer", Type: types.MemoryTypeCore},
		{Content: "yesterday the API changed", Type: types.MemoryTypeTemporal},
		{Content: "we discussed design patterns", Type: types.MemoryTypeEpisodic},
		{Content: "how to deploy step by step", Type: types.MemoryTypeProcedural},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.RouteWrite(entries[i%len(entries)])
	}
}

func BenchmarkRouter_RouteWrite_AutoClassify(b *testing.B) {
	r := routing.NewRouter()

	entries := []*types.MemoryEntry{
		{Content: "a simple fact about Go"},
		{Content: "auth depends on database"},
		{Content: "i am a developer"},
		{Content: "yesterday the API changed"},
		{Content: "we discussed design patterns"},
		{Content: "how to deploy step by step"},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Reset type so classification runs each time
		e := *entries[i%len(entries)]
		e.Type = ""
		_ = r.RouteWrite(&e)
	}
}

func BenchmarkRouter_RouteRead(b *testing.B) {
	r := routing.NewRouter()

	reqs := []*types.SearchRequest{
		{Query: "test"},
		{Query: "test", Sources: []types.MemorySource{types.SourceMem0}},
		{Query: "test", Types: []types.MemoryType{types.MemoryTypeFact}},
		{Query: "test", Sources: []types.MemorySource{
			types.SourceMem0, types.SourceCognee,
		}},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.RouteRead(reqs[i%len(reqs)])
	}
}

func BenchmarkMemoryStoreAdapter_Add(b *testing.B) {
	u, _, _ := newBenchUnified(0)
	adapter := provider.NewMemoryStoreAdapter(u)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mem := &modstore.Memory{
			ID:      fmt.Sprintf("bench-adapter-%d", i),
			Content: fmt.Sprintf("adapter benchmark content %d", i),
			Scope:   modstore.ScopeUser,
			Metadata: map[string]any{
				"key": "value",
			},
		}
		_ = adapter.Add(ctx, mem)
	}
}

func BenchmarkMemoryStoreAdapter_Search(b *testing.B) {
	u, _, _ := newBenchUnified(100)
	adapter := provider.NewMemoryStoreAdapter(u)
	ctx := context.Background()

	opts := &modstore.SearchOptions{TopK: 10}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = adapter.Search(ctx, "benchmark entry", opts)
	}
}

func BenchmarkUnifiedProvider_Get(b *testing.B) {
	u, _, _ := newBenchUnified(100)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("mem0-%d", i%100)
		_, _ = u.Get(ctx, id)
	}
}

func BenchmarkUnifiedProvider_Health(b *testing.B) {
	u, _, _ := newBenchUnified(0)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = u.Health(ctx)
	}
}
