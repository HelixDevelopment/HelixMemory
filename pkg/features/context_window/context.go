// Package context_window implements Adaptive Context Window Engineering
// for HelixMemory. It dynamically manages context windows by selecting
// the most relevant memories to fit within token limits.
package context_window

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"digital.vasic.helixmemory/pkg/types"
)

// ContextWindow represents a managed context window with token-aware
// memory selection.
type ContextWindow struct {
	provider   types.MemoryProvider
	maxTokens  int
	reserved   int // tokens reserved for system prompt + response
}

// NewContextWindow creates an adaptive context window manager.
func NewContextWindow(provider types.MemoryProvider, maxTokens int) *ContextWindow {
	return &ContextWindow{
		provider:  provider,
		maxTokens: maxTokens,
		reserved:  2000, // Reserve for system prompt + response
	}
}

// ContextBlock represents a piece of context to inject.
type ContextBlock struct {
	Content    string                 `json:"content"`
	Source     types.MemorySource     `json:"source"`
	Priority   float64               `json:"priority"`
	TokenCount int                    `json:"token_count"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ManagedContext is the output — a token-budgeted set of memory blocks.
type ManagedContext struct {
	Blocks      []*ContextBlock `json:"blocks"`
	TotalTokens int             `json:"total_tokens"`
	MaxTokens   int             `json:"max_tokens"`
	Utilization float64         `json:"utilization"` // 0.0-1.0
	Dropped     int             `json:"dropped"`
}

// Build constructs an optimally-packed context window from memories.
func (cw *ContextWindow) Build(ctx context.Context, query string, userID string) (*ManagedContext, error) {
	req := &types.SearchRequest{
		Query:  query,
		UserID: userID,
		TopK:   50, // Fetch more than we need, we'll pack optimally
	}

	result, err := cw.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("context window search: %w", err)
	}

	// Convert to context blocks with token estimates
	blocks := make([]*ContextBlock, 0, len(result.Entries))
	for _, entry := range result.Entries {
		block := &ContextBlock{
			Content:    entry.Content,
			Source:     entry.Source,
			Priority:   entry.Relevance*0.6 + entry.Confidence*0.4,
			TokenCount: estimateTokens(entry.Content),
			Metadata:   entry.Metadata,
		}
		blocks = append(blocks, block)
	}

	// Sort by priority (highest first)
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Priority > blocks[j].Priority
	})

	// Greedy packing within token budget
	available := cw.maxTokens - cw.reserved
	managed := &ManagedContext{
		MaxTokens: cw.maxTokens,
	}

	for _, block := range blocks {
		if managed.TotalTokens+block.TokenCount <= available {
			managed.Blocks = append(managed.Blocks, block)
			managed.TotalTokens += block.TokenCount
		} else {
			managed.Dropped++
		}
	}

	if cw.maxTokens > 0 {
		managed.Utilization = float64(managed.TotalTokens) / float64(cw.maxTokens)
	}

	return managed, nil
}

// SetReserved sets the number of tokens reserved for non-memory content.
func (cw *ContextWindow) SetReserved(tokens int) {
	cw.reserved = tokens
}

// estimateTokens provides a rough token count estimate (4 chars per token).
func estimateTokens(content string) int {
	words := len(strings.Fields(content))
	chars := len(content)
	// Average of word-based and char-based estimates
	return (words*4/3 + chars/4) / 2
}
