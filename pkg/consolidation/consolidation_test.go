package consolidation

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

type testProvider struct {
	name    types.MemorySource
	entries []*types.MemoryEntry
}

func (p *testProvider) Name() types.MemorySource { return p.name }
func (p *testProvider) Add(_ context.Context, e *types.MemoryEntry) error {
	p.entries = append(p.entries, e)
	return nil
}
func (p *testProvider) Search(_ context.Context, _ *types.SearchRequest) (*types.SearchResult, error) {
	return &types.SearchResult{Entries: p.entries}, nil
}
func (p *testProvider) Get(_ context.Context, id string) (*types.MemoryEntry, error) {
	for _, e := range p.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (p *testProvider) Update(_ context.Context, _ *types.MemoryEntry) error { return nil }
func (p *testProvider) Delete(_ context.Context, _ string) error              { return nil }
func (p *testProvider) GetHistory(_ context.Context, _ string, limit int) ([]*types.MemoryEntry, error) {
	if limit > len(p.entries) {
		limit = len(p.entries)
	}
	return p.entries[:limit], nil
}
func (p *testProvider) Health(_ context.Context) error { return nil }

func TestEngine_RunOnce(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ConsolidationBatchSize = 100

	engine := NewEngine(cfg)

	provider := &testProvider{
		name: types.SourceMem0,
		entries: []*types.MemoryEntry{
			{ID: "1", Content: "memory one", CreatedAt: time.Now()},
			{ID: "2", Content: "memory two", CreatedAt: time.Now()},
			{ID: "3", Content: "memory three", CreatedAt: time.Now()},
		},
	}

	engine.RegisterProvider(provider)

	ctx := context.Background()
	err := engine.RunOnce(ctx)
	require.NoError(t, err)

	stats := engine.GetStats()
	assert.Equal(t, 1, stats.TotalRuns)
	assert.Equal(t, 3, stats.MemoriesProcessed)
	assert.Equal(t, 3, stats.Consolidated)
}

func TestEngine_RunOnce_WithDuplicates(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	// Two providers with same ID entry
	p1 := &testProvider{
		name: types.SourceMem0,
		entries: []*types.MemoryEntry{
			{ID: "dup-1", Content: "same content", CreatedAt: time.Now()},
		},
	}
	p2 := &testProvider{
		name: types.SourceCognee,
		entries: []*types.MemoryEntry{
			{ID: "dup-1", Content: "same content", CreatedAt: time.Now()},
		},
	}

	engine.RegisterProvider(p1)
	engine.RegisterProvider(p2)

	ctx := context.Background()
	err := engine.RunOnce(ctx)
	require.NoError(t, err)

	stats := engine.GetStats()
	assert.Equal(t, 2, stats.MemoriesProcessed)
	assert.Equal(t, 1, stats.Deduplicated) // One duplicate removed
	assert.Equal(t, 1, stats.Consolidated)
}

func TestEngine_RunOnce_NoProviders(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	ctx := context.Background()
	err := engine.RunOnce(ctx)
	assert.NoError(t, err)
}

func TestEngine_StartStop(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ConsolidationEnabled = true
	cfg.ConsolidationInterval = 50 * time.Millisecond

	engine := NewEngine(cfg)
	engine.RegisterProvider(&testProvider{
		name:    types.SourceMem0,
		entries: []*types.MemoryEntry{{ID: "1", Content: "test"}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := engine.Start(ctx)
	require.NoError(t, err)
	assert.True(t, engine.IsRunning())

	// Wait for at least one consolidation cycle
	time.Sleep(100 * time.Millisecond)

	engine.Stop()
	assert.False(t, engine.IsRunning())

	stats := engine.GetStats()
	assert.GreaterOrEqual(t, stats.TotalRuns, 1)
}

func TestEngine_Start_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ConsolidationEnabled = false

	engine := NewEngine(cfg)

	ctx := context.Background()
	err := engine.Start(ctx)
	assert.NoError(t, err)
	assert.False(t, engine.IsRunning())
}

func TestEngine_Start_AlreadyRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ConsolidationEnabled = true
	cfg.ConsolidationInterval = 1 * time.Hour

	engine := NewEngine(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := engine.Start(ctx)
	require.NoError(t, err)
	defer engine.Stop()

	err = engine.Start(ctx)
	assert.Error(t, err)
}

func TestEngine_GetConsolidationStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	status := engine.GetConsolidationStatus()
	assert.NotNil(t, status)
	assert.False(t, status.Running)
	assert.Equal(t, 0, status.MemoriesProcessed)
}

func TestEngine_GetConsolidationStatus_AfterRun(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)
	engine.RegisterProvider(&testProvider{
		name:    types.SourceMem0,
		entries: []*types.MemoryEntry{{ID: "1", Content: "test"}},
	})

	ctx := context.Background()
	_ = engine.RunOnce(ctx)

	status := engine.GetConsolidationStatus()
	assert.Equal(t, 1, status.MemoriesProcessed)
	assert.Equal(t, 1, status.Consolidated)
}
