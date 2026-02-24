// Package cross_project implements Cross-Project Knowledge Transfer
// for HelixMemory. It enables transferring learned patterns, conventions,
// and domain knowledge between different projects.
package cross_project

import (
	"context"
	"fmt"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// TransferableKnowledge represents knowledge that can be transferred.
type TransferableKnowledge struct {
	ID          string               `json:"id"`
	SourceProject string             `json:"source_project"`
	Category    string               `json:"category"` // pattern, convention, architecture
	Entries     []*types.MemoryEntry `json:"entries"`
	Confidence  float64              `json:"confidence"`
}

// TransferResult reports on a knowledge transfer operation.
type TransferResult struct {
	SourceProject string `json:"source_project"`
	TargetProject string `json:"target_project"`
	Transferred   int    `json:"transferred"`
	Skipped       int    `json:"skipped"`
	Failed        int    `json:"failed"`
	Duration      time.Duration `json:"duration"`
}

// Transferor manages cross-project knowledge transfer.
type Transferor struct {
	provider types.MemoryProvider
}

// NewTransferor creates a cross-project knowledge transferor.
func NewTransferor(provider types.MemoryProvider) *Transferor {
	return &Transferor{provider: provider}
}

// IdentifyTransferable finds knowledge in a source project that could
// benefit a target project.
func (t *Transferor) IdentifyTransferable(ctx context.Context, sourceProject, targetContext string) ([]*TransferableKnowledge, error) {
	// Search for patterns from the source project
	req := &types.SearchRequest{
		Query: fmt.Sprintf("[DNA:%s]", sourceProject),
		TopK:  50,
		Types: []types.MemoryType{types.MemoryTypeProcedural, types.MemoryTypeGraph},
	}

	result, err := t.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("identify transferable: %w", err)
	}

	// Group by category
	categories := make(map[string][]*types.MemoryEntry)
	for _, entry := range result.Entries {
		cat := "general"
		if c, ok := entry.Metadata["dna_type"].(string); ok {
			cat = c
		}
		categories[cat] = append(categories[cat], entry)
	}

	var knowledge []*TransferableKnowledge
	for cat, entries := range categories {
		tk := &TransferableKnowledge{
			ID:            uuid.New().String(),
			SourceProject: sourceProject,
			Category:      cat,
			Entries:       entries,
		}

		// Calculate average confidence
		total := 0.0
		for _, e := range entries {
			total += e.Confidence
		}
		if len(entries) > 0 {
			tk.Confidence = total / float64(len(entries))
		}

		knowledge = append(knowledge, tk)
	}

	return knowledge, nil
}

// Transfer applies knowledge from one project to another.
func (t *Transferor) Transfer(ctx context.Context, sourceProject, targetProject string) (*TransferResult, error) {
	start := time.Now()

	knowledge, err := t.IdentifyTransferable(ctx, sourceProject, targetProject)
	if err != nil {
		return nil, err
	}

	result := &TransferResult{
		SourceProject: sourceProject,
		TargetProject: targetProject,
	}

	for _, tk := range knowledge {
		for _, entry := range tk.Entries {
			// Create a new entry for the target project
			newEntry := &types.MemoryEntry{
				ID:      uuid.New().String(),
				Content: entry.Content,
				Type:    entry.Type,
				Source:  types.SourceFusion,
				Metadata: map[string]interface{}{
					"transferred_from":  sourceProject,
					"transferred_to":    targetProject,
					"transfer_category": tk.Category,
					"original_id":       entry.ID,
				},
				Confidence: entry.Confidence * 0.9, // Slight confidence reduction
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			if err := t.provider.Add(ctx, newEntry); err != nil {
				result.Failed++
				continue
			}
			result.Transferred++
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}
