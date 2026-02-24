package procedural

import (
	"context"
	"fmt"
	"strings"
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

func TestManager_LearnProcedure(t *testing.T) {
	tests := []struct {
		name        string
		procName    string
		description string
		steps       []Step
	}{
		{
			name:        "learn a procedure with 3 steps",
			procName:    "deploy-app",
			description: "Deploy the application to production",
			steps: []Step{
				{Order: 1, Action: "Build binary", Description: "Compile"},
				{Order: 2, Action: "Run tests", Description: "Verify"},
				{Order: 3, Action: "Push to prod", Description: "Deploy"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			mgr := NewManager(prov)

			proc, err := mgr.LearnProcedure(
				context.Background(), tc.procName, tc.description, tc.steps,
			)
			require.NoError(t, err)
			require.NotNil(t, proc)

			assert.NotEmpty(t, proc.ID)
			assert.Equal(t, tc.procName, proc.Name)
			assert.Equal(t, tc.description, proc.Description)
			assert.Len(t, proc.Steps, len(tc.steps))
			assert.Equal(t, 1.0, proc.SuccessRate)
			assert.Equal(t, 1, proc.UsageCount)

			// Verify stored in provider
			assert.Len(t, prov.entries, 1)
			stored := prov.entries[proc.ID]
			require.NotNil(t, stored)
			assert.Equal(t, types.MemoryTypeProcedural, stored.Type)
		})
	}
}

func TestManager_LearnProcedure_WithCommands(t *testing.T) {
	tests := []struct {
		name     string
		steps    []Step
		expectIn string
	}{
		{
			name: "steps with commands include cmd format",
			steps: []Step{
				{Order: 1, Action: "Build", Command: "make build"},
				{Order: 2, Action: "Test", Command: "make test"},
			},
			expectIn: "(cmd: make build)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			mgr := NewManager(prov)

			proc, err := mgr.LearnProcedure(
				context.Background(), "build-flow", "Build flow", tc.steps,
			)
			require.NoError(t, err)

			stored := prov.entries[proc.ID]
			require.NotNil(t, stored)
			assert.True(t, strings.Contains(stored.Content, tc.expectIn),
				"content should contain %q, got: %s", tc.expectIn, stored.Content)
		})
	}
}

func TestManager_LearnProcedure_AddError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.addErr = fmt.Errorf("disk full")
	mgr := NewManager(prov)

	_, err := mgr.LearnProcedure(
		context.Background(), "fail", "Will fail",
		[]Step{{Order: 1, Action: "step1"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store procedure")
}

func TestManager_FindProcedure(t *testing.T) {
	tests := []struct {
		name        string
		seedEntries []*types.MemoryEntry
		query       string
		expectNames []string
	}{
		{
			name: "find procedures by query",
			seedEntries: []*types.MemoryEntry{
				{
					ID:      "p1",
					Content: "[PROCEDURE:deploy] Deploy to prod",
					Type:    types.MemoryTypeProcedural,
					Metadata: map[string]interface{}{
						"procedure_name": "deploy",
						"success_rate":   0.95,
					},
				},
			},
			query:       "deploy",
			expectNames: []string{"deploy"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for _, e := range tc.seedEntries {
				prov.entries[e.ID] = e
			}
			mgr := NewManager(prov)

			procs, err := mgr.FindProcedure(context.Background(), tc.query)
			require.NoError(t, err)
			require.Len(t, procs, len(tc.expectNames))

			for i, name := range tc.expectNames {
				assert.Equal(t, name, procs[i].Name)
				assert.Greater(t, procs[i].SuccessRate, 0.0)
			}
		})
	}
}

func TestManager_FindProcedure_NoResults(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	mgr := NewManager(prov)

	procs, err := mgr.FindProcedure(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, procs)
}

func TestManager_RecordOutcome_Success(t *testing.T) {
	tests := []struct {
		name           string
		initialRate    float64
		initialCount   float64
		success        bool
		expectedRate   float64
		expectedCount  int
	}{
		{
			name:          "record success applies EMA",
			initialRate:   0.8,
			initialCount:  5,
			success:       true,
			expectedRate:  0.82, // 0.8*0.9 + 0.1
			expectedCount: 6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			prov.entries["proc-1"] = &types.MemoryEntry{
				ID:      "proc-1",
				Content: "[PROCEDURE:test] Test procedure",
				Type:    types.MemoryTypeProcedural,
				Metadata: map[string]interface{}{
					"success_rate": tc.initialRate,
					"usage_count":  tc.initialCount,
				},
			}
			mgr := NewManager(prov)

			err := mgr.RecordOutcome(context.Background(), "proc-1", tc.success)
			require.NoError(t, err)

			updated := prov.entries["proc-1"]
			rate, _ := updated.Metadata["success_rate"].(float64)
			count, _ := updated.Metadata["usage_count"].(int)

			assert.InDelta(t, tc.expectedRate, rate, 0.001)
			assert.Equal(t, tc.expectedCount, count)
		})
	}
}

func TestManager_RecordOutcome_Failure(t *testing.T) {
	tests := []struct {
		name         string
		initialRate  float64
		initialCount float64
		expectedRate float64
	}{
		{
			name:         "record failure decreases rate via EMA",
			initialRate:  0.8,
			initialCount: 5,
			expectedRate: 0.72, // 0.8 * 0.9
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			prov.entries["proc-2"] = &types.MemoryEntry{
				ID:      "proc-2",
				Content: "[PROCEDURE:test] Test",
				Type:    types.MemoryTypeProcedural,
				Metadata: map[string]interface{}{
					"success_rate": tc.initialRate,
					"usage_count":  tc.initialCount,
				},
			}
			mgr := NewManager(prov)

			err := mgr.RecordOutcome(context.Background(), "proc-2", false)
			require.NoError(t, err)

			updated := prov.entries["proc-2"]
			rate, _ := updated.Metadata["success_rate"].(float64)
			assert.InDelta(t, tc.expectedRate, rate, 0.001)
		})
	}
}

func TestManager_RecordOutcome_NotFound(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	mgr := NewManager(prov)

	err := mgr.RecordOutcome(context.Background(), "nonexistent", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get procedure")
}
