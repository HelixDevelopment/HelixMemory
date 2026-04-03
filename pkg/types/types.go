// Package types provides core types for the HelixMemory unified cognitive
// memory engine. It defines memory entries, types, sources, and the provider
// interface that all backends implement.
package types

import (
	"context"
	"time"
)

// MemoryType categorizes memories by their cognitive function.
type MemoryType string

const (
	// MemoryTypeFact represents extracted facts (Mem0 primary).
	MemoryTypeFact MemoryType = "fact"
	// MemoryTypeGraph represents knowledge graph entries (Cognee primary).
	MemoryTypeGraph MemoryType = "graph"
	// MemoryTypeCore represents core/persona memory (Letta primary).
	MemoryTypeCore MemoryType = "core"
	// MemoryTypeTemporal represents time-aware memories (Graphiti primary).
	MemoryTypeTemporal MemoryType = "temporal"
	// MemoryTypeEpisodic represents conversation/event memories.
	MemoryTypeEpisodic MemoryType = "episodic"
	// MemoryTypeProcedural represents learned workflows and procedures.
	MemoryTypeProcedural MemoryType = "procedural"
	// MemoryTypeSemantic represents general semantic knowledge.
	MemoryTypeSemantic MemoryType = "semantic"
)

// MemorySource identifies which backend produced or owns a memory.
type MemorySource string

const (
	// SourceMem0 indicates the Mem0 backend.
	SourceMem0 MemorySource = "mem0"
	// SourceCognee indicates the Cognee backend.
	SourceCognee MemorySource = "cognee"
	// SourceLetta indicates the Letta backend.
	SourceLetta MemorySource = "letta"
	// SourceGraphiti indicates the Graphiti temporal layer.
	SourceGraphiti MemorySource = "graphiti"
	// SourceFusion indicates the memory was fused from multiple sources.
	SourceFusion MemorySource = "fusion"
)

// MemoryEntry represents a unified memory record from any backend.
type MemoryEntry struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Type        MemoryType             `json:"type"`
	Source      MemorySource           `json:"source"`
	Confidence  float64                `json:"confidence"`
	Relevance   float64                `json:"relevance"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Embedding   []float32              `json:"embedding,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	AgentID     string                 `json:"agent_id,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
	AccessCount int                    `json:"access_count"`
	LastAccess  time.Time              `json:"last_access"`
}

// CoreMemoryBlock represents an editable in-context memory block (Letta-style).
type CoreMemoryBlock struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	Limit   int    `json:"limit"`
	AgentID string `json:"agent_id,omitempty"`
}

// SearchRequest defines a unified search query across all backends.
type SearchRequest struct {
	Query     string                 `json:"query"`
	UserID    string                 `json:"user_id,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	Types     []MemoryType           `json:"types,omitempty"`
	Sources   []MemorySource         `json:"sources,omitempty"`
	TopK      int                    `json:"top_k"`
	MinScore  float64                `json:"min_score"`
	TimeRange *TimeRange             `json:"time_range,omitempty"`
	Filter    map[string]interface{} `json:"filter,omitempty"`
}

// TimeRange restricts results to a time window.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SearchResult contains search results with metadata.
type SearchResult struct {
	Entries  []*MemoryEntry `json:"entries"`
	Total    int            `json:"total"`
	Duration time.Duration  `json:"duration"`
	Sources  []MemorySource `json:"sources_queried"`
}

// FusionResult contains fused results from multiple memory systems.
type FusionResult struct {
	Entries     []*MemoryEntry         `json:"entries"`
	Total       int                    `json:"total"`
	Duration    time.Duration          `json:"duration"`
	Query       string                 `json:"query"`
	Sources     []MemorySource         `json:"sources"`
	SourceStats map[MemorySource]int   `json:"source_stats"`
	FusionScore float64                `json:"fusion_score"`
}

// FusionStats provides statistics about the fusion engine.
type FusionStats struct {
	CogneeHealthy bool                 `json:"cognee_healthy"`
	Mem0Healthy   bool                 `json:"mem0_healthy"`
	LettaHealthy  bool                 `json:"letta_healthy"`
	CogneeCount   int64                `json:"cognee_count"`
	Mem0Count     int64                `json:"mem0_count"`
	LettaCount    int64                `json:"letta_count"`
	Timestamp     time.Time            `json:"timestamp"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// MemoryProvider defines the interface that all memory backends implement.
// This is the contract that Mem0, Cognee, Letta, and Graphiti clients fulfill.
type MemoryProvider interface {
	// Name returns the provider identifier.
	Name() MemorySource

	// Add stores a new memory entry.
	Add(ctx context.Context, entry *MemoryEntry) error

	// Search returns memories matching the request.
	Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)

	// Get retrieves a memory by ID.
	Get(ctx context.Context, id string) (*MemoryEntry, error)

	// Update modifies an existing memory.
	Update(ctx context.Context, entry *MemoryEntry) error

	// Delete removes a memory by ID.
	Delete(ctx context.Context, id string) error

	// GetHistory returns memory history for a user.
	GetHistory(ctx context.Context, userID string, limit int) ([]*MemoryEntry, error)

	// Health checks if the backend is available.
	Health(ctx context.Context) error
}

// CoreMemoryProvider extends MemoryProvider with Letta-style core memory.
type CoreMemoryProvider interface {
	MemoryProvider

	// GetCoreMemory retrieves core memory blocks for an agent.
	GetCoreMemory(ctx context.Context, agentID string) ([]*CoreMemoryBlock, error)

	// UpdateCoreMemory updates a core memory block.
	UpdateCoreMemory(ctx context.Context, agentID string, block *CoreMemoryBlock) error
}

// ConsolidationProvider extends MemoryProvider with sleep-time compute.
type ConsolidationProvider interface {
	MemoryProvider

	// TriggerConsolidation starts sleep-time memory consolidation.
	TriggerConsolidation(ctx context.Context, userID string) error

	// GetConsolidationStatus returns the current consolidation state.
	GetConsolidationStatus(ctx context.Context) (*ConsolidationStatus, error)
}

// ConsolidationStatus reports on sleep-time compute progress.
type ConsolidationStatus struct {
	Running           bool          `json:"running"`
	LastRun           time.Time     `json:"last_run"`
	MemoriesProcessed int           `json:"memories_processed"`
	Deduplicated      int           `json:"deduplicated"`
	Consolidated      int           `json:"consolidated"`
	Duration          time.Duration `json:"duration"`
}

// TemporalProvider extends MemoryProvider with time-aware queries.
type TemporalProvider interface {
	MemoryProvider

	// SearchTemporal queries memories with temporal reasoning.
	SearchTemporal(ctx context.Context, query string, at time.Time) ([]*MemoryEntry, error)

	// GetTimeline returns a chronological view of memories.
	GetTimeline(ctx context.Context, userID string, start, end time.Time) ([]*MemoryEntry, error)

	// InvalidateAt marks memories as invalid at a point in time.
	InvalidateAt(ctx context.Context, id string, at time.Time) error
}

// DefaultSearchRequest returns sensible defaults for search.
func DefaultSearchRequest(query string) *SearchRequest {
	return &SearchRequest{
		Query:    query,
		TopK:     10,
		MinScore: 0.0,
	}
}
