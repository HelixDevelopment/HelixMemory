// Package provider contains the UnifiedMemoryProvider and its adapter
// to the digital.vasic.memory module's MemoryStore interface.
package provider

import (
	"context"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	modstore "digital.vasic.memory/pkg/store"

	"github.com/google/uuid"
)

// MemoryStoreAdapter adapts UnifiedMemoryProvider to implement the
// digital.vasic.memory MemoryStore interface. This is the bridge that allows
// HelixMemory to be used as a drop-in replacement for the default Memory module.
type MemoryStoreAdapter struct {
	provider *UnifiedMemoryProvider
}

// NewMemoryStoreAdapter wraps a UnifiedMemoryProvider with the MemoryStore interface.
func NewMemoryStoreAdapter(provider *UnifiedMemoryProvider) *MemoryStoreAdapter {
	return &MemoryStoreAdapter{provider: provider}
}

// Add stores a new memory via the MemoryStore interface.
func (a *MemoryStoreAdapter) Add(ctx context.Context, memory *modstore.Memory) error {
	entry := moduleMemoryToEntry(memory)
	return a.provider.Add(ctx, entry)
}

// Search returns memories matching the query.
func (a *MemoryStoreAdapter) Search(ctx context.Context, query string, opts *modstore.SearchOptions) ([]*modstore.Memory, error) {
	req := &types.SearchRequest{
		Query: query,
		TopK:  10,
	}
	if opts != nil {
		req.TopK = opts.TopK
		req.MinScore = opts.MinScore
		if opts.Scope != "" {
			req.Filter = map[string]interface{}{
				"scope": string(opts.Scope),
			}
		}
		if opts.TimeRange != nil {
			req.TimeRange = &types.TimeRange{
				Start: opts.TimeRange.Start,
				End:   opts.TimeRange.End,
			}
		}
	}

	result, err := a.provider.Search(ctx, req)
	if err != nil {
		return nil, err
	}

	memories := make([]*modstore.Memory, len(result.Entries))
	for i, entry := range result.Entries {
		memories[i] = entryToModuleMemory(entry)
	}
	return memories, nil
}

// Get retrieves a memory by ID.
func (a *MemoryStoreAdapter) Get(ctx context.Context, id string) (*modstore.Memory, error) {
	entry, err := a.provider.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return entryToModuleMemory(entry), nil
}

// Update modifies an existing memory.
func (a *MemoryStoreAdapter) Update(ctx context.Context, memory *modstore.Memory) error {
	entry := moduleMemoryToEntry(memory)
	return a.provider.Update(ctx, entry)
}

// Delete removes a memory by ID.
func (a *MemoryStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.provider.Delete(ctx, id)
}

// List returns memories matching the scope and options.
func (a *MemoryStoreAdapter) List(ctx context.Context, scope modstore.Scope, opts *modstore.ListOptions) ([]*modstore.Memory, error) {
	req := &types.SearchRequest{
		Query: "*",
		TopK:  100,
	}
	if opts != nil {
		req.TopK = opts.Limit
	}
	if scope != "" {
		if req.Filter == nil {
			req.Filter = make(map[string]interface{})
		}
		req.Filter["scope"] = string(scope)
	}

	result, err := a.provider.Search(ctx, req)
	if err != nil {
		return nil, err
	}

	memories := make([]*modstore.Memory, len(result.Entries))
	for i, entry := range result.Entries {
		memories[i] = entryToModuleMemory(entry)
	}
	return memories, nil
}

// moduleMemoryToEntry converts a digital.vasic.memory Memory to a HelixMemory entry.
func moduleMemoryToEntry(m *modstore.Memory) *types.MemoryEntry {
	if m == nil {
		return nil
	}

	entry := &types.MemoryEntry{
		ID:        m.ID,
		Content:   m.Content,
		Type:      types.MemoryTypeFact,
		Source:    types.SourceFusion,
		Relevance: m.Score,
		Metadata:  make(map[string]interface{}),
		Embedding: m.Embedding,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	// Preserve module metadata
	for k, v := range m.Metadata {
		entry.Metadata[k] = v
	}
	entry.Metadata["scope"] = string(m.Scope)

	return entry
}

// entryToModuleMemory converts a HelixMemory entry to a digital.vasic.memory Memory.
func entryToModuleMemory(e *types.MemoryEntry) *modstore.Memory {
	if e == nil {
		return nil
	}

	m := &modstore.Memory{
		ID:        e.ID,
		Content:   e.Content,
		Metadata:  make(map[string]any),
		Scope:     modstore.ScopeUser,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		Score:     e.Relevance,
		Embedding: e.Embedding,
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}

	// Copy metadata
	for k, v := range e.Metadata {
		m.Metadata[k] = v
	}

	// Extract scope from metadata
	if scope, ok := e.Metadata["scope"].(string); ok {
		m.Scope = modstore.Scope(scope)
	}

	// Embed source info
	m.Metadata["helix_source"] = string(e.Source)
	m.Metadata["helix_type"] = string(e.Type)
	m.Metadata["helix_confidence"] = e.Confidence

	return m
}
