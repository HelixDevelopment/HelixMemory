// Package snapshots implements Memory Snapshots & Rollback for HelixMemory.
// It enables point-in-time snapshots of the memory state for backup,
// comparison, and rollback operations.
package snapshots

import (
	"context"
	"fmt"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// Snapshot represents a point-in-time capture of memory state.
type Snapshot struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Entries   []*types.MemoryEntry `json:"entries"`
	CreatedAt time.Time           `json:"created_at"`
	EntryCount int               `json:"entry_count"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Manager manages memory snapshots and rollback.
type Manager struct {
	mu        sync.RWMutex
	provider  types.MemoryProvider
	snapshots map[string]*Snapshot
}

// NewManager creates a snapshot manager.
func NewManager(provider types.MemoryProvider) *Manager {
	return &Manager{
		provider:  provider,
		snapshots: make(map[string]*Snapshot),
	}
}

// CreateSnapshot captures the current memory state.
func (m *Manager) CreateSnapshot(ctx context.Context, name string) (*Snapshot, error) {
	result, err := m.provider.Search(ctx, &types.SearchRequest{
		Query: "*",
		TopK:  10000,
	})
	if err != nil {
		return nil, fmt.Errorf("create snapshot: search failed: %w", err)
	}

	snapshot := &Snapshot{
		ID:         uuid.New().String(),
		Name:       name,
		Entries:    make([]*types.MemoryEntry, len(result.Entries)),
		CreatedAt:  time.Now(),
		EntryCount: len(result.Entries),
	}

	// Deep copy entries
	for i, entry := range result.Entries {
		copied := *entry
		if entry.Metadata != nil {
			copied.Metadata = make(map[string]interface{})
			for k, v := range entry.Metadata {
				copied.Metadata[k] = v
			}
		}
		snapshot.Entries[i] = &copied
	}

	m.mu.Lock()
	m.snapshots[snapshot.ID] = snapshot
	m.mu.Unlock()

	return snapshot, nil
}

// GetSnapshot retrieves a snapshot by ID.
func (m *Manager) GetSnapshot(id string) (*Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap, ok := m.snapshots[id]
	if !ok {
		return nil, fmt.Errorf("snapshot %s not found", id)
	}
	return snap, nil
}

// ListSnapshots returns all snapshots.
func (m *Manager) ListSnapshots() []*Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Snapshot, 0, len(m.snapshots))
	for _, s := range m.snapshots {
		// Return without entries for listing
		result = append(result, &Snapshot{
			ID:         s.ID,
			Name:       s.Name,
			CreatedAt:  s.CreatedAt,
			EntryCount: s.EntryCount,
		})
	}
	return result
}

// DeleteSnapshot removes a snapshot.
func (m *Manager) DeleteSnapshot(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.snapshots[id]; !ok {
		return fmt.Errorf("snapshot %s not found", id)
	}
	delete(m.snapshots, id)
	return nil
}

// CompareSnapshots shows differences between two snapshots.
func (m *Manager) CompareSnapshots(id1, id2 string) (*SnapshotDiff, error) {
	m.mu.RLock()
	snap1, ok1 := m.snapshots[id1]
	snap2, ok2 := m.snapshots[id2]
	m.mu.RUnlock()

	if !ok1 {
		return nil, fmt.Errorf("snapshot %s not found", id1)
	}
	if !ok2 {
		return nil, fmt.Errorf("snapshot %s not found", id2)
	}

	ids1 := make(map[string]*types.MemoryEntry)
	for _, e := range snap1.Entries {
		ids1[e.ID] = e
	}

	ids2 := make(map[string]*types.MemoryEntry)
	for _, e := range snap2.Entries {
		ids2[e.ID] = e
	}

	diff := &SnapshotDiff{
		Snapshot1: id1,
		Snapshot2: id2,
	}

	for id, entry := range ids1 {
		if _, ok := ids2[id]; !ok {
			diff.Removed = append(diff.Removed, entry)
		}
	}

	for id, entry := range ids2 {
		if _, ok := ids1[id]; !ok {
			diff.Added = append(diff.Added, entry)
		}
	}

	diff.TotalChanges = len(diff.Added) + len(diff.Removed)
	return diff, nil
}

// SnapshotDiff represents differences between two snapshots.
type SnapshotDiff struct {
	Snapshot1    string               `json:"snapshot_1"`
	Snapshot2    string               `json:"snapshot_2"`
	Added        []*types.MemoryEntry `json:"added"`
	Removed      []*types.MemoryEntry `json:"removed"`
	TotalChanges int                  `json:"total_changes"`
}
