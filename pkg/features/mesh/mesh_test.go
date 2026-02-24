package mesh

import (
	"context"
	"fmt"
	"sync"
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
	mu        sync.Mutex
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
	if p.addErr != nil {
		return p.addErr
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Search(
	_ context.Context, req *types.SearchRequest,
) (*types.SearchResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[id]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found")
}

func (p *testProvider) Update(_ context.Context, entry *types.MemoryEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.entries[entry.ID]; !ok {
		return fmt.Errorf("not found")
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
	p.mu.Lock()
	defer p.mu.Unlock()
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

func TestMesh_RegisterAgent(t *testing.T) {
	tests := []struct {
		name    string
		agents  []*AgentInfo
		wantLen int
	}{
		{
			name: "register single agent",
			agents: []*AgentInfo{
				{ID: "agent-1", Name: "Alpha", Role: "researcher"},
			},
			wantLen: 1,
		},
		{
			name: "register multiple agents",
			agents: []*AgentInfo{
				{ID: "agent-1", Name: "Alpha", Role: "researcher"},
				{ID: "agent-2", Name: "Beta", Role: "reviewer"},
			},
			wantLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			m := NewMesh(prov)

			for _, a := range tc.agents {
				m.RegisterAgent(a)
			}

			agents := m.GetAgents()
			assert.Len(t, agents, tc.wantLen)
		})
	}
}

func TestMesh_UnregisterAgent(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	m := NewMesh(prov)

	m.RegisterAgent(&AgentInfo{ID: "agent-1", Name: "Alpha"})
	require.Len(t, m.GetAgents(), 1)

	m.UnregisterAgent("agent-1")
	assert.Len(t, m.GetAgents(), 0)
}

func TestMesh_ShareMemory(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		agentTeam  string
		scope      MeshScope
		expectMeta map[string]string
	}{
		{
			name:      "share with global scope",
			agentID:   "agent-1",
			agentTeam: "team-red",
			scope:     ScopeGlobal,
			expectMeta: map[string]string{
				"mesh_scope": "global",
				"mesh_owner": "agent-1",
				"mesh_team":  "team-red",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			m := NewMesh(prov)

			m.RegisterAgent(&AgentInfo{
				ID:   tc.agentID,
				Name: "TestAgent",
				Team: tc.agentTeam,
			})

			entry := &types.MemoryEntry{
				ID:      "mem-1",
				Content: "shared knowledge",
				Type:    types.MemoryTypeFact,
				Source:  types.SourceFusion,
			}

			err := m.ShareMemory(context.Background(), tc.agentID, entry, tc.scope)
			require.NoError(t, err)

			for k, v := range tc.expectMeta {
				assert.Equal(t, v, entry.Metadata[k],
					"metadata %q should be %q", k, v)
			}
			assert.Equal(t, tc.agentID, entry.AgentID)

			// Verify provider received the entry
			assert.Len(t, prov.entries, 1)
		})
	}
}

func TestMesh_ShareMemory_UnregisteredAgent(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	m := NewMesh(prov)

	entry := &types.MemoryEntry{
		ID:      "mem-1",
		Content: "orphan data",
	}

	err := m.ShareMemory(context.Background(), "unknown-agent", entry, ScopeGlobal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestMesh_ShareMemory_NilMetadata(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	m := NewMesh(prov)
	m.RegisterAgent(&AgentInfo{ID: "a1", Name: "Agent1", Team: "t1"})

	entry := &types.MemoryEntry{
		ID:       "mem-nil",
		Content:  "test nil metadata",
		Metadata: nil, // explicitly nil
	}

	err := m.ShareMemory(context.Background(), "a1", entry, ScopeGlobal)
	require.NoError(t, err)
	require.NotNil(t, entry.Metadata, "metadata should be initialized")
	assert.Equal(t, "global", entry.Metadata["mesh_scope"])
}

func TestMesh_SearchMeshMemories_Global(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	m := NewMesh(prov)

	m.RegisterAgent(&AgentInfo{ID: "a1", Name: "Alpha", Team: "t1"})
	m.RegisterAgent(&AgentInfo{ID: "a2", Name: "Beta", Team: "t2"})

	// Store global-scoped entries
	prov.entries["g1"] = &types.MemoryEntry{
		ID:      "g1",
		Content: "global knowledge",
		Metadata: map[string]interface{}{
			"mesh_scope": "global",
			"mesh_owner": "a1",
			"mesh_team":  "t1",
		},
	}

	results, err := m.SearchMeshMemories(context.Background(), "a2", "knowledge", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1, "global entries visible to all agents")
}

func TestMesh_SearchMeshMemories_Private(t *testing.T) {
	tests := []struct {
		name      string
		searchAs  string
		expectLen int
	}{
		{
			name:      "owner can see private entries",
			searchAs:  "a1",
			expectLen: 1,
		},
		{
			name:      "non-owner cannot see private entries",
			searchAs:  "a2",
			expectLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			m := NewMesh(prov)

			m.RegisterAgent(&AgentInfo{ID: "a1", Name: "Alpha", Team: "t1"})
			m.RegisterAgent(&AgentInfo{ID: "a2", Name: "Beta", Team: "t2"})

			prov.entries["p1"] = &types.MemoryEntry{
				ID:      "p1",
				Content: "private secret",
				Metadata: map[string]interface{}{
					"mesh_scope": "private",
					"mesh_owner": "a1",
					"mesh_team":  "t1",
				},
			}

			results, err := m.SearchMeshMemories(
				context.Background(), tc.searchAs, "secret", 10,
			)
			require.NoError(t, err)
			assert.Len(t, results, tc.expectLen)
		})
	}
}

func TestMesh_SearchMeshMemories_Team(t *testing.T) {
	tests := []struct {
		name      string
		searchAs  string
		expectLen int
	}{
		{
			name:      "same team can see team entries",
			searchAs:  "a2",
			expectLen: 1,
		},
		{
			name:      "different team cannot see team entries",
			searchAs:  "a3",
			expectLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			m := NewMesh(prov)

			m.RegisterAgent(&AgentInfo{ID: "a1", Name: "Alpha", Team: "team-red"})
			m.RegisterAgent(&AgentInfo{ID: "a2", Name: "Beta", Team: "team-red"})
			m.RegisterAgent(&AgentInfo{ID: "a3", Name: "Gamma", Team: "team-blue"})

			prov.entries["t1"] = &types.MemoryEntry{
				ID:      "t1",
				Content: "team knowledge",
				Metadata: map[string]interface{}{
					"mesh_scope": "team",
					"mesh_owner": "a1",
					"mesh_team":  "team-red",
				},
			}

			results, err := m.SearchMeshMemories(
				context.Background(), tc.searchAs, "knowledge", 10,
			)
			require.NoError(t, err)
			assert.Len(t, results, tc.expectLen)
		})
	}
}

func TestMesh_SearchMeshMemories_UnregisteredAgent(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	m := NewMesh(prov)

	_, err := m.SearchMeshMemories(
		context.Background(), "unknown", "query", 10,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestMesh_TransferKnowledge(t *testing.T) {
	tests := []struct {
		name            string
		seedEntries     map[string]*types.MemoryEntry
		fromAgent       string
		toAgent         string
		expectTransfer  int
	}{
		{
			name: "transfer memories from agent1 to agent2",
			seedEntries: map[string]*types.MemoryEntry{
				"k1": {
					ID:      "k1",
					Content: "important fact",
					Metadata: map[string]interface{}{
						"mesh_scope": "global",
						"mesh_owner": "agent-1",
						"mesh_team":  "team-a",
					},
				},
				"k2": {
					ID:      "k2",
					Content: "another fact",
					Metadata: map[string]interface{}{
						"mesh_scope": "global",
						"mesh_owner": "agent-1",
						"mesh_team":  "team-a",
					},
				},
			},
			fromAgent:      "agent-1",
			toAgent:        "agent-2",
			expectTransfer: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for k, v := range tc.seedEntries {
				prov.entries[k] = v
			}

			m := NewMesh(prov)
			m.RegisterAgent(&AgentInfo{
				ID: tc.fromAgent, Name: "From", Team: "team-a",
			})
			m.RegisterAgent(&AgentInfo{
				ID: tc.toAgent, Name: "To", Team: "team-a",
			})

			transferred, err := m.TransferKnowledge(
				context.Background(), tc.fromAgent, tc.toAgent, "fact", 10,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.expectTransfer, transferred)

			// Verify new entries have correct metadata
			found := 0
			for _, entry := range prov.entries {
				if owner, _ := entry.Metadata["mesh_owner"].(string); owner == tc.toAgent {
					from, _ := entry.Metadata["mesh_transferred_from"].(string)
					assert.Equal(t, tc.fromAgent, from)
					assert.Equal(t, "private",
						entry.Metadata["mesh_scope"])
					assert.Equal(t, tc.toAgent, entry.AgentID)
					found++
				}
			}
			assert.Equal(t, tc.expectTransfer, found)
		})
	}
}

func TestMesh_Concurrency(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	m := NewMesh(prov)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrently register agents
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			m.RegisterAgent(&AgentInfo{
				ID:   fmt.Sprintf("agent-%d", idx),
				Name: fmt.Sprintf("Agent%d", idx),
				Role: "worker",
				Team: "concurrent",
			})
		}(i)
	}

	// Concurrently unregister agents (some may not exist yet)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			m.UnregisterAgent(fmt.Sprintf("agent-%d", idx))
		}(i)
	}

	wg.Wait()

	// No panic = success; just verify GetAgents doesn't race
	_ = m.GetAgents()
}
