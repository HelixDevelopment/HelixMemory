package quality_loop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is an in-memory mock of types.MemoryProvider for unit tests.
type testProvider struct {
	name      types.MemorySource
	entries   map[string]*types.MemoryEntry
	healthy   bool
	addErr    error
	searchErr error
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
	if p.addErr != nil {
		return p.addErr
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Search(
	_ context.Context, req *types.SearchRequest,
) (*types.SearchResult, error) {
	if p.searchErr != nil {
		return nil, p.searchErr
	}
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		entries = append(entries, e)
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

func (p *testProvider) GetHistory(
	_ context.Context, _ string, limit int,
) ([]*types.MemoryEntry, error) {
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

func TestLoop_Analyze(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	now := time.Now()
	staleThreshold := 30 * 24 * time.Hour

	// High-confidence, recent entry
	prov.entries["e1"] = &types.MemoryEntry{
		ID:         "e1",
		Content:    "high confidence fact",
		Confidence: 0.9,
		CreatedAt:  now.Add(-1 * time.Hour),
	}
	// High-confidence, recent entry
	prov.entries["e2"] = &types.MemoryEntry{
		ID:         "e2",
		Content:    "another high confidence fact",
		Confidence: 0.8,
		CreatedAt:  now.Add(-2 * time.Hour),
	}
	// Low-confidence entry
	prov.entries["e3"] = &types.MemoryEntry{
		ID:         "e3",
		Content:    "uncertain fact",
		Confidence: 0.1,
		CreatedAt:  now.Add(-1 * time.Hour),
	}
	// Stale entry (older than threshold)
	prov.entries["e4"] = &types.MemoryEntry{
		ID:         "e4",
		Content:    "old stale fact",
		Confidence: 0.5,
		CreatedAt:  now.Add(-staleThreshold - 1*time.Hour),
	}

	cfg := DefaultConfig()
	loop := NewLoop(prov, cfg)

	report, err := loop.Analyze(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 4, report.TotalMemories)
	assert.Equal(t, 2, report.HighConfidence)
	assert.Equal(t, 1, report.LowConfidence)
	assert.Equal(t, 1, report.Stale)

	expectedAvg := (0.9 + 0.8 + 0.1 + 0.5) / 4.0
	assert.InDelta(t, expectedAvg, report.AverageConfidence, 0.01)

	// Should have recommended actions for stale and low-confidence
	require.GreaterOrEqual(t, len(report.RecommendedActions), 2)

	actionTypes := make(map[string]bool)
	for _, a := range report.RecommendedActions {
		actionTypes[a.Type] = true
	}
	assert.True(t, actionTypes["prune"], "should recommend pruning stale entries")
	assert.True(t, actionTypes["validate"],
		"should recommend validating low-confidence entries")
}

func TestLoop_Analyze_Empty(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	cfg := DefaultConfig()
	loop := NewLoop(prov, cfg)

	report, err := loop.Analyze(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 0, report.TotalMemories)
	assert.Equal(t, 0, report.HighConfidence)
	assert.Equal(t, 0, report.LowConfidence)
	assert.Equal(t, 0, report.Stale)
	assert.InDelta(t, 0.0, report.AverageConfidence, 0.001)
	assert.Empty(t, report.RecommendedActions)
}

func TestLoop_Analyze_SearchError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.searchErr = fmt.Errorf("backend unavailable")

	cfg := DefaultConfig()
	loop := NewLoop(prov, cfg)

	_, err := loop.Analyze(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quality scan")
}

func TestLoop_Start_Stop(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	cfg := DefaultConfig()
	cfg.Interval = 100 * time.Millisecond
	loop := NewLoop(prov, cfg)

	ctx := context.Background()
	err := loop.Start(ctx)
	require.NoError(t, err)

	// Verify running
	loop.mu.RLock()
	running := loop.running
	loop.mu.RUnlock()
	assert.True(t, running, "loop should be running after Start")

	loop.Stop()

	loop.mu.RLock()
	running = loop.running
	loop.mu.RUnlock()
	assert.False(t, running, "loop should not be running after Stop")
}

func TestLoop_Start_AlreadyRunning(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	cfg := DefaultConfig()
	cfg.Interval = 100 * time.Millisecond
	loop := NewLoop(prov, cfg)

	ctx := context.Background()
	err := loop.Start(ctx)
	require.NoError(t, err)
	defer loop.Stop()

	err = loop.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestLoop_Start_Disabled(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	cfg := DefaultConfig()
	cfg.Enabled = false
	loop := NewLoop(prov, cfg)

	ctx := context.Background()
	err := loop.Start(ctx)
	require.NoError(t, err, "Start should return nil when disabled")

	loop.mu.RLock()
	running := loop.running
	loop.mu.RUnlock()
	assert.False(t, running, "loop should not be running when disabled")
}

func TestLoop_GetStats(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	cfg := DefaultConfig()
	cfg.Interval = 50 * time.Millisecond
	loop := NewLoop(prov, cfg)

	stats := loop.GetStats()
	assert.Equal(t, 0, stats.TotalScans)
	assert.Equal(t, 0, stats.TotalPruned)
	assert.Equal(t, 0, stats.TotalRefreshed)
	assert.True(t, stats.LastScanAt.IsZero())
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, 1*time.Hour, cfg.Interval)
	assert.Equal(t, 30*24*time.Hour, cfg.StaleThreshold)
	assert.InDelta(t, 0.3, cfg.LowConfidenceLimit, 0.001)
	assert.Equal(t, 500, cfg.MaxMemoriesPerScan)
}
