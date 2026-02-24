package temporal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is a minimal in-memory MemoryProvider for unit testing.
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

func TestReasoner_WhatWasTrue(t *testing.T) {
	now := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-2 * time.Hour)

	tests := []struct {
		name        string
		entries     map[string]*types.MemoryEntry
		queryTime   time.Time
		expectCount int
		expectActive bool
	}{
		{
			name: "returns entries created before query time, sorted by ValidAt",
			entries: map[string]*types.MemoryEntry{
				"e1": {
					ID:        "e1",
					Content:   "fact one",
					CreatedAt: earlier,
					Metadata:  map[string]interface{}{},
				},
				"e2": {
					ID:        "e2",
					Content:   "fact two",
					CreatedAt: earlier.Add(30 * time.Minute),
					Metadata:  map[string]interface{}{},
				},
			},
			queryTime:    now,
			expectCount:  2,
			expectActive: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceGraphiti)
			prov.entries = tc.entries

			r := NewReasoner(prov)
			results, err := r.WhatWasTrue(
				context.Background(), "fact", tc.queryTime,
			)
			require.NoError(t, err)
			assert.Len(t, results, tc.expectCount)

			// Verify sorted by ValidAt
			for i := 1; i < len(results); i++ {
				assert.True(t,
					results[i-1].ValidAt.Before(results[i].ValidAt) ||
						results[i-1].ValidAt.Equal(results[i].ValidAt),
					"entries should be sorted by ValidAt")
			}

			// Verify IsActive for non-invalidated
			for _, entry := range results {
				assert.Equal(t, tc.expectActive, entry.IsActive)
			}
		})
	}
}

func TestReasoner_WhatWasTrue_WithInvalidation(t *testing.T) {
	now := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-3 * time.Hour)
	invalidatedAt := now.Add(-1 * time.Hour) // before query time

	tests := []struct {
		name         string
		invalidAt    time.Time
		queryTime    time.Time
		expectActive bool
	}{
		{
			name:         "entry invalidated before query time is inactive",
			invalidAt:    invalidatedAt,
			queryTime:    now,
			expectActive: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceGraphiti)
			prov.entries["inv1"] = &types.MemoryEntry{
				ID:        "inv1",
				Content:   "obsolete fact",
				CreatedAt: earlier,
				Metadata: map[string]interface{}{
					"invalid_at": tc.invalidAt,
				},
			}

			r := NewReasoner(prov)
			results, err := r.WhatWasTrue(
				context.Background(), "fact", tc.queryTime,
			)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			assert.Equal(t, tc.expectActive, results[0].IsActive)
		})
	}
}

func TestReasoner_WhatWasTrue_SearchError(t *testing.T) {
	prov := newTestProvider(types.SourceGraphiti)
	prov.searchErr = fmt.Errorf("database down")

	r := NewReasoner(prov)
	_, err := r.WhatWasTrue(
		context.Background(), "query", time.Now(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "temporal search")
}

func TestReasoner_BuildTimeline(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		entries     map[string]*types.MemoryEntry
		expectLen   int
		expectStart time.Time
		expectEnd   time.Time
	}{
		{
			name: "timeline with entries in range",
			entries: map[string]*types.MemoryEntry{
				"t1": {
					ID:        "t1",
					Content:   "event one",
					CreatedAt: start.Add(24 * time.Hour),
					Metadata:  map[string]interface{}{},
				},
				"t2": {
					ID:        "t2",
					Content:   "event two",
					CreatedAt: start.Add(48 * time.Hour),
					Metadata:  map[string]interface{}{},
				},
			},
			expectLen:   2,
			expectStart: start,
			expectEnd:   end,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceGraphiti)
			prov.entries = tc.entries

			r := NewReasoner(prov)
			timeline, err := r.BuildTimeline(
				context.Background(), "event", start, end,
			)
			require.NoError(t, err)
			assert.Len(t, timeline.Entries, tc.expectLen)
			assert.Equal(t, tc.expectStart, timeline.Start)
			assert.Equal(t, tc.expectEnd, timeline.End)
			assert.Equal(t, end.Sub(start), timeline.Duration)

			// Verify sorted
			for i := 1; i < len(timeline.Entries); i++ {
				assert.True(t,
					timeline.Entries[i-1].ValidAt.Before(
						timeline.Entries[i].ValidAt) ||
						timeline.Entries[i-1].ValidAt.Equal(
							timeline.Entries[i].ValidAt))
			}
		})
	}
}

func TestReasoner_BuildTimeline_Empty(t *testing.T) {
	prov := newTestProvider(types.SourceGraphiti)
	r := NewReasoner(prov)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	timeline, err := r.BuildTimeline(
		context.Background(), "nothing", start, end,
	)
	require.NoError(t, err)
	assert.Empty(t, timeline.Entries)
	assert.Equal(t, start, timeline.Start)
	assert.Equal(t, end, timeline.End)
}

func TestReasoner_WhatChanged(t *testing.T) {
	from := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)

	createdDuring := from.Add(48 * time.Hour)   // Jan 12
	invalidatedDuring := from.Add(72 * time.Hour) // Jan 13

	tests := []struct {
		name      string
		entries   map[string]*types.MemoryEntry
		expectLen int
	}{
		{
			name: "entries created or invalidated in range are returned",
			entries: map[string]*types.MemoryEntry{
				"c1": {
					ID:        "c1",
					Content:   "new fact",
					CreatedAt: createdDuring,
					Metadata:  map[string]interface{}{},
				},
				"c2": {
					ID:        "c2",
					Content:   "old fact invalidated",
					CreatedAt: from.Add(-48 * time.Hour), // before range
					Metadata: map[string]interface{}{
						"invalid_at": invalidatedDuring,
					},
				},
			},
			// c1: validAt (Jan 12) is between from and to => changed
			// c2: validAt (Jan 8) is NOT between from and to,
			//     but invalidAt (Jan 13) is => also changed
			expectLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceGraphiti)
			prov.entries = tc.entries

			r := NewReasoner(prov)
			changed, err := r.WhatChanged(
				context.Background(), "fact", from, to,
			)
			require.NoError(t, err)
			assert.Len(t, changed, tc.expectLen)
		})
	}
}
