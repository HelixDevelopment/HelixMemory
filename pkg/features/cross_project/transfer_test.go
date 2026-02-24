package cross_project

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

func TestTransferor_IdentifyTransferable(t *testing.T) {
	tests := []struct {
		name           string
		entries        []*types.MemoryEntry
		wantCategories int
		wantAvgConf    map[string]float64
	}{
		{
			name: "grouped by dna_type category",
			entries: []*types.MemoryEntry{
				{
					ID:         "e1",
					Content:    "singleton pattern",
					Type:       types.MemoryTypeProcedural,
					Confidence: 0.8,
					Metadata:   map[string]interface{}{"dna_type": "pattern"},
				},
				{
					ID:         "e2",
					Content:    "factory pattern",
					Type:       types.MemoryTypeProcedural,
					Confidence: 0.6,
					Metadata:   map[string]interface{}{"dna_type": "pattern"},
				},
				{
					ID:         "e3",
					Content:    "use camelCase",
					Type:       types.MemoryTypeGraph,
					Confidence: 0.9,
					Metadata:   map[string]interface{}{"dna_type": "convention"},
				},
			},
			wantCategories: 2,
			wantAvgConf: map[string]float64{
				"pattern":    0.7,
				"convention": 0.9,
			},
		},
		{
			name: "entries without dna_type default to general",
			entries: []*types.MemoryEntry{
				{
					ID:         "e1",
					Content:    "some knowledge",
					Type:       types.MemoryTypeProcedural,
					Confidence: 0.5,
					Metadata:   map[string]interface{}{},
				},
			},
			wantCategories: 1,
			wantAvgConf: map[string]float64{
				"general": 0.5,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for _, e := range tc.entries {
				prov.entries[e.ID] = e
			}

			tr := NewTransferor(prov)
			result, err := tr.IdentifyTransferable(
				context.Background(), "projectA", "projectB",
			)
			require.NoError(t, err)
			assert.Len(t, result, tc.wantCategories)

			for _, tk := range result {
				expected, ok := tc.wantAvgConf[tk.Category]
				if ok {
					assert.InDelta(t, expected, tk.Confidence, 0.01,
						"category %s confidence mismatch", tk.Category)
				}
			}
		})
	}
}

func TestTransferor_IdentifyTransferable_SearchError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.searchErr = fmt.Errorf("connection refused")

	tr := NewTransferor(prov)
	_, err := tr.IdentifyTransferable(
		context.Background(), "projectA", "projectB",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identify transferable")
}

func TestTransferor_Transfer(t *testing.T) {
	tests := []struct {
		name           string
		entries        []*types.MemoryEntry
		wantTransferred int
	}{
		{
			name: "transfers entries from source to target",
			entries: []*types.MemoryEntry{
				{
					ID:         "src-1",
					Content:    "observer pattern",
					Type:       types.MemoryTypeProcedural,
					Source:     types.SourceMem0,
					Confidence: 0.8,
					Metadata:   map[string]interface{}{"dna_type": "pattern"},
				},
				{
					ID:         "src-2",
					Content:    "use interfaces",
					Type:       types.MemoryTypeGraph,
					Source:     types.SourceCognee,
					Confidence: 1.0,
					Metadata:   map[string]interface{}{"dna_type": "convention"},
				},
			},
			wantTransferred: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for _, e := range tc.entries {
				prov.entries[e.ID] = e
			}

			tr := NewTransferor(prov)
			result, err := tr.Transfer(
				context.Background(), "projectA", "projectB",
			)
			require.NoError(t, err)

			assert.Equal(t, tc.wantTransferred, result.Transferred)
			assert.Equal(t, "projectA", result.SourceProject)
			assert.Equal(t, "projectB", result.TargetProject)
			assert.Equal(t, 0, result.Failed)

			// Verify transferred entries have correct metadata and
			// reduced confidence.
			for _, entry := range prov.entries {
				meta := entry.Metadata
				if meta == nil {
					continue
				}
				if _, ok := meta["transferred_from"]; !ok {
					continue
				}
				assert.Equal(t, "projectA", meta["transferred_from"])
				assert.Equal(t, "projectB", meta["transferred_to"])
				assert.NotEmpty(t, meta["transfer_category"])
				assert.NotEmpty(t, meta["original_id"])

				// Confidence should be original * 0.9
				origID, _ := meta["original_id"].(string)
				for _, src := range tc.entries {
					if src.ID == origID {
						assert.InDelta(t, src.Confidence*0.9,
							entry.Confidence, 0.001)
					}
				}
			}
		})
	}
}

func TestTransferor_Transfer_Empty(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	// No entries in provider

	tr := NewTransferor(prov)
	result, err := tr.Transfer(
		context.Background(), "emptyProject", "targetProject",
	)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Transferred)
	assert.Equal(t, 0, result.Failed)
}

func TestTransferor_Transfer_AddFailure(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["src-1"] = &types.MemoryEntry{
		ID:         "src-1",
		Content:    "some knowledge",
		Type:       types.MemoryTypeProcedural,
		Confidence: 0.7,
		Metadata:   map[string]interface{}{"dna_type": "pattern"},
	}

	tr := NewTransferor(prov)

	// After IdentifyTransferable succeeds, make Add fail for new entries
	prov.addErr = fmt.Errorf("storage full")

	result, err := tr.Transfer(
		context.Background(), "projectA", "projectB",
	)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Transferred)
	assert.Greater(t, result.Failed, 0)
}
