// Package mesh implements the Multi-Agent Memory Mesh for HelixMemory.
// It enables multiple AI agents to share memories through a unified mesh,
// with scope isolation, access control, and cross-agent knowledge transfer.
package mesh

import (
	"context"
	"fmt"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/types"
)

// MeshScope defines visibility boundaries for shared memories.
type MeshScope string

const (
	// ScopePrivate means only the owning agent can access.
	ScopePrivate MeshScope = "private"
	// ScopeTeam means agents in the same team can access.
	ScopeTeam MeshScope = "team"
	// ScopeGlobal means all agents can access.
	ScopeGlobal MeshScope = "global"
)

// AgentInfo describes an agent in the mesh.
type AgentInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	Team     string    `json:"team,omitempty"`
}

// Mesh manages shared memory across multiple agents.
type Mesh struct {
	mu       sync.RWMutex
	agents   map[string]*AgentInfo
	provider types.MemoryProvider
}

// NewMesh creates a multi-agent memory mesh.
func NewMesh(provider types.MemoryProvider) *Mesh {
	return &Mesh{
		agents:   make(map[string]*AgentInfo),
		provider: provider,
	}
}

// RegisterAgent adds an agent to the mesh.
func (m *Mesh) RegisterAgent(agent *AgentInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	agent.JoinedAt = time.Now()
	m.agents[agent.ID] = agent
}

// UnregisterAgent removes an agent from the mesh.
func (m *Mesh) UnregisterAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, agentID)
}

// GetAgents returns all registered agents.
func (m *Mesh) GetAgents() []*AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	return agents
}

// ShareMemory stores a memory with mesh scope visibility.
func (m *Mesh) ShareMemory(ctx context.Context, agentID string, entry *types.MemoryEntry, scope MeshScope) error {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("mesh: agent %s not registered", agentID)
	}

	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}
	entry.Metadata["mesh_scope"] = string(scope)
	entry.Metadata["mesh_owner"] = agentID
	entry.Metadata["mesh_team"] = agent.Team
	entry.AgentID = agentID

	return m.provider.Add(ctx, entry)
}

// SearchMeshMemories searches for memories visible to an agent based on scope.
func (m *Mesh) SearchMeshMemories(ctx context.Context, agentID string, query string, topK int) ([]*types.MemoryEntry, error) {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("mesh: agent %s not registered", agentID)
	}

	req := &types.SearchRequest{
		Query:   query,
		AgentID: agentID,
		TopK:    topK,
	}

	result, err := m.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mesh: search failed: %w", err)
	}

	// Filter by scope visibility
	var visible []*types.MemoryEntry
	for _, entry := range result.Entries {
		if m.isVisible(entry, agentID, agent.Team) {
			visible = append(visible, entry)
		}
	}

	return visible, nil
}

// isVisible checks if a memory entry is visible to an agent.
func (m *Mesh) isVisible(entry *types.MemoryEntry, agentID, team string) bool {
	scope, _ := entry.Metadata["mesh_scope"].(string)
	owner, _ := entry.Metadata["mesh_owner"].(string)
	entryTeam, _ := entry.Metadata["mesh_team"].(string)

	switch MeshScope(scope) {
	case ScopePrivate:
		return owner == agentID
	case ScopeTeam:
		return entryTeam == team
	case ScopeGlobal:
		return true
	default:
		// No scope set — visible to all (backwards compat)
		return true
	}
}

// TransferKnowledge copies relevant memories from one agent to another.
func (m *Mesh) TransferKnowledge(ctx context.Context, fromAgentID, toAgentID string, query string, topK int) (int, error) {
	entries, err := m.SearchMeshMemories(ctx, fromAgentID, query, topK)
	if err != nil {
		return 0, fmt.Errorf("mesh: transfer search: %w", err)
	}

	transferred := 0
	for _, entry := range entries {
		shared := *entry
		shared.AgentID = toAgentID
		if shared.Metadata == nil {
			shared.Metadata = make(map[string]interface{})
		}
		shared.Metadata["mesh_transferred_from"] = fromAgentID
		shared.Metadata["mesh_scope"] = string(ScopePrivate)
		shared.Metadata["mesh_owner"] = toAgentID

		if err := m.provider.Add(ctx, &shared); err != nil {
			continue
		}
		transferred++
	}

	return transferred, nil
}
