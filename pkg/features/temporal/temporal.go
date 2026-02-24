// Package temporal implements Temporal Reasoning via Graphiti for HelixMemory.
// It enables bi-temporal queries, edge invalidation, timeline construction,
// and "what was true at time T?" style reasoning.
package temporal

import (
	"context"
	"fmt"
	"sort"
	"time"

	"digital.vasic.helixmemory/pkg/types"
)

// TimelineEntry represents a memory at a specific point in time.
type TimelineEntry struct {
	Memory    *types.MemoryEntry `json:"memory"`
	ValidAt   time.Time          `json:"valid_at"`
	InvalidAt *time.Time         `json:"invalid_at,omitempty"`
	IsActive  bool               `json:"is_active"`
}

// Timeline represents a chronological sequence of memories.
type Timeline struct {
	Entries  []*TimelineEntry `json:"entries"`
	Start    time.Time        `json:"start"`
	End      time.Time        `json:"end"`
	Duration time.Duration    `json:"duration"`
}

// Reasoner performs temporal reasoning over memories.
type Reasoner struct {
	provider types.MemoryProvider
}

// NewReasoner creates a temporal reasoning engine.
func NewReasoner(provider types.MemoryProvider) *Reasoner {
	return &Reasoner{provider: provider}
}

// WhatWasTrue returns memories that were valid at a specific point in time.
func (r *Reasoner) WhatWasTrue(ctx context.Context, query string, at time.Time) ([]*TimelineEntry, error) {
	req := &types.SearchRequest{
		Query: query,
		TopK:  50,
		TimeRange: &types.TimeRange{
			Start: at.Add(-365 * 24 * time.Hour),
			End:   at.Add(1 * time.Second),
		},
	}

	result, err := r.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("temporal search: %w", err)
	}

	var entries []*TimelineEntry
	for _, memory := range result.Entries {
		validAt := memory.CreatedAt
		if v, ok := memory.Metadata["valid_at"].(time.Time); ok {
			validAt = v
		}

		entry := &TimelineEntry{
			Memory:  memory,
			ValidAt: validAt,
		}

		// Check if invalidated
		if inv, ok := memory.Metadata["invalid_at"].(time.Time); ok {
			entry.InvalidAt = &inv
			entry.IsActive = at.Before(inv)
		} else {
			entry.IsActive = true
		}

		// Only include if valid at the requested time
		if validAt.Before(at) || validAt.Equal(at) {
			entries = append(entries, entry)
		}
	}

	// Sort by valid_at
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ValidAt.Before(entries[j].ValidAt)
	})

	return entries, nil
}

// BuildTimeline constructs a chronological timeline of memories.
func (r *Reasoner) BuildTimeline(ctx context.Context, query string, start, end time.Time) (*Timeline, error) {
	req := &types.SearchRequest{
		Query: query,
		TopK:  100,
		TimeRange: &types.TimeRange{
			Start: start,
			End:   end,
		},
	}

	result, err := r.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("timeline search: %w", err)
	}

	var entries []*TimelineEntry
	for _, memory := range result.Entries {
		validAt := memory.CreatedAt
		if v, ok := memory.Metadata["valid_at"].(time.Time); ok {
			validAt = v
		}

		entry := &TimelineEntry{
			Memory:   memory,
			ValidAt:  validAt,
			IsActive: true,
		}

		if inv, ok := memory.Metadata["invalid_at"].(time.Time); ok {
			entry.InvalidAt = &inv
			entry.IsActive = time.Now().Before(inv)
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ValidAt.Before(entries[j].ValidAt)
	})

	return &Timeline{
		Entries:  entries,
		Start:    start,
		End:      end,
		Duration: end.Sub(start),
	}, nil
}

// WhatChanged returns memories that changed between two time points.
func (r *Reasoner) WhatChanged(ctx context.Context, query string, from, to time.Time) ([]*TimelineEntry, error) {
	timeline, err := r.BuildTimeline(ctx, query, from, to)
	if err != nil {
		return nil, err
	}

	var changed []*TimelineEntry
	for _, entry := range timeline.Entries {
		if entry.ValidAt.After(from) && entry.ValidAt.Before(to) {
			changed = append(changed, entry)
		}
		if entry.InvalidAt != nil && entry.InvalidAt.After(from) && entry.InvalidAt.Before(to) {
			changed = append(changed, entry)
		}
	}

	return changed, nil
}
