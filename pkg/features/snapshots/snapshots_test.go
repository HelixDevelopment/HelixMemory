package snapshots

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

func TestManager_CreateSnapshot(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "fact one",
		Type:    types.MemoryTypeFact,
	}
	prov.entries["e2"] = &types.MemoryEntry{
		ID:      "e2",
		Content: "fact two",
		Type:    types.MemoryTypeFact,
	}

	mgr := NewManager(prov)
	snap, err := mgr.CreateSnapshot(context.Background(), "test-snapshot")
	require.NoError(t, err)

	assert.NotEmpty(t, snap.ID)
	assert.Equal(t, "test-snapshot", snap.Name)
	assert.Equal(t, 2, snap.EntryCount)
	assert.Len(t, snap.Entries, 2)
	assert.False(t, snap.CreatedAt.IsZero())
}

func TestManager_CreateSnapshot_SearchError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.searchErr = fmt.Errorf("search backend down")

	mgr := NewManager(prov)
	_, err := mgr.CreateSnapshot(context.Background(), "failing-snapshot")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create snapshot")
}

func TestManager_GetSnapshot(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "test data",
	}

	mgr := NewManager(prov)
	created, err := mgr.CreateSnapshot(context.Background(), "retrievable")
	require.NoError(t, err)

	retrieved, err := mgr.GetSnapshot(created.ID)
	require.NoError(t, err)

	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Name, retrieved.Name)
	assert.Equal(t, created.EntryCount, retrieved.EntryCount)
}

func TestManager_GetSnapshot_NotFound(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	mgr := NewManager(prov)

	_, err := mgr.GetSnapshot("nonexistent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestManager_ListSnapshots(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "data",
	}

	mgr := NewManager(prov)
	ctx := context.Background()

	_, err := mgr.CreateSnapshot(ctx, "snap-1")
	require.NoError(t, err)
	_, err = mgr.CreateSnapshot(ctx, "snap-2")
	require.NoError(t, err)
	_, err = mgr.CreateSnapshot(ctx, "snap-3")
	require.NoError(t, err)

	list := mgr.ListSnapshots()
	assert.Len(t, list, 3)

	// Verify listing contains only metadata, not entries
	for _, s := range list {
		assert.NotEmpty(t, s.ID)
		assert.NotEmpty(t, s.Name)
		assert.Nil(t, s.Entries,
			"listing should not include entries (only metadata)")
	}
}

func TestManager_DeleteSnapshot(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "data",
	}

	mgr := NewManager(prov)
	snap, err := mgr.CreateSnapshot(context.Background(), "to-delete")
	require.NoError(t, err)

	err = mgr.DeleteSnapshot(snap.ID)
	require.NoError(t, err)

	_, err = mgr.GetSnapshot(snap.ID)
	require.Error(t, err, "snapshot should be gone after deletion")
}

func TestManager_DeleteSnapshot_NotFound(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	mgr := NewManager(prov)

	err := mgr.DeleteSnapshot("nonexistent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestManager_CompareSnapshots(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	ctx := context.Background()
	mgr := NewManager(prov)

	// First snapshot with entries e1, e2
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "first entry",
	}
	prov.entries["e2"] = &types.MemoryEntry{
		ID:      "e2",
		Content: "second entry",
	}
	snap1, err := mgr.CreateSnapshot(ctx, "snap-v1")
	require.NoError(t, err)

	// Second snapshot with entries e2, e3 (e1 removed, e3 added)
	delete(prov.entries, "e1")
	prov.entries["e3"] = &types.MemoryEntry{
		ID:      "e3",
		Content: "third entry",
	}
	snap2, err := mgr.CreateSnapshot(ctx, "snap-v2")
	require.NoError(t, err)

	diff, err := mgr.CompareSnapshots(snap1.ID, snap2.ID)
	require.NoError(t, err)

	// e3 is in snap2 but not snap1 => Added
	assert.NotEmpty(t, diff.Added, "e3 should appear in Added")
	// e1 is in snap1 but not snap2 => Removed
	assert.NotEmpty(t, diff.Removed, "e1 should appear in Removed")

	assert.Equal(t, len(diff.Added)+len(diff.Removed), diff.TotalChanges)
	assert.GreaterOrEqual(t, diff.TotalChanges, 2)
}

func TestManager_CompareSnapshots_NotFound(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "data",
	}

	mgr := NewManager(prov)
	ctx := context.Background()

	snap1, err := mgr.CreateSnapshot(ctx, "exists")
	require.NoError(t, err)

	_, err = mgr.CompareSnapshots(snap1.ID, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_, err = mgr.CompareSnapshots("nonexistent", snap1.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestManager_DeepCopy(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	original := &types.MemoryEntry{
		ID:      "dc-1",
		Content: "original content",
		Metadata: map[string]interface{}{
			"key": "original_value",
		},
	}
	prov.entries["dc-1"] = original

	mgr := NewManager(prov)
	snap, err := mgr.CreateSnapshot(context.Background(), "deep-copy-test")
	require.NoError(t, err)

	// Modify the original entry after snapshot creation
	original.Content = "modified content"
	original.Metadata["key"] = "modified_value"
	original.Metadata["new_key"] = "new_value"

	// Verify the snapshot entry is unchanged (deep copy)
	require.Len(t, snap.Entries, 1)
	snapEntry := snap.Entries[0]

	assert.Equal(t, "original content", snapEntry.Content,
		"snapshot entry content should not change when original is modified")
	assert.Equal(t, "original_value", snapEntry.Metadata["key"],
		"snapshot entry metadata should not change when original is modified")
	_, hasNewKey := snapEntry.Metadata["new_key"]
	assert.False(t, hasNewKey,
		"snapshot metadata should not have keys added after snapshot creation")
}
