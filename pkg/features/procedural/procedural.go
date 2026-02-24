// Package procedural implements Procedural Memory for HelixMemory.
// It captures learned workflows, debugging strategies, deployment procedures,
// and other "how-to" knowledge from user interactions.
package procedural

import (
	"context"
	"fmt"
	"strings"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// Procedure represents a learned workflow or procedure.
type Procedure struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []Step   `json:"steps"`
	Tags        []string `json:"tags,omitempty"`
	SuccessRate float64  `json:"success_rate"`
	UsageCount  int      `json:"usage_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Step represents a single step in a procedure.
type Step struct {
	Order       int    `json:"order"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Expected    string `json:"expected,omitempty"`
}

// Manager manages procedural memories — learned workflows and strategies.
type Manager struct {
	provider types.MemoryProvider
}

// NewManager creates a procedural memory manager.
func NewManager(provider types.MemoryProvider) *Manager {
	return &Manager{provider: provider}
}

// LearnProcedure extracts and stores a procedure from observed steps.
func (m *Manager) LearnProcedure(ctx context.Context, name, description string, steps []Step) (*Procedure, error) {
	proc := &Procedure{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Steps:       steps,
		SuccessRate: 1.0,
		UsageCount:  1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Build content for memory storage
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[PROCEDURE:%s] %s\n", name, description))
	for _, step := range steps {
		sb.WriteString(fmt.Sprintf("Step %d: %s", step.Order, step.Action))
		if step.Command != "" {
			sb.WriteString(fmt.Sprintf(" (cmd: %s)", step.Command))
		}
		sb.WriteString("\n")
	}

	entry := &types.MemoryEntry{
		ID:      proc.ID,
		Content: sb.String(),
		Type:    types.MemoryTypeProcedural,
		Source:  types.SourceFusion,
		Metadata: map[string]interface{}{
			"procedure_name": name,
			"step_count":     len(steps),
			"success_rate":   proc.SuccessRate,
			"usage_count":    proc.UsageCount,
		},
		Confidence: 0.85,
		CreatedAt:  proc.CreatedAt,
		UpdatedAt:  proc.UpdatedAt,
	}

	if err := m.provider.Add(ctx, entry); err != nil {
		return nil, fmt.Errorf("store procedure: %w", err)
	}

	return proc, nil
}

// FindProcedure searches for a procedure matching the query.
func (m *Manager) FindProcedure(ctx context.Context, query string) ([]*Procedure, error) {
	req := &types.SearchRequest{
		Query: query,
		TopK:  5,
		Types: []types.MemoryType{types.MemoryTypeProcedural},
	}

	result, err := m.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search procedures: %w", err)
	}

	var procedures []*Procedure
	for _, entry := range result.Entries {
		proc := &Procedure{
			ID:        entry.ID,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		}
		if name, ok := entry.Metadata["procedure_name"].(string); ok {
			proc.Name = name
		}
		if rate, ok := entry.Metadata["success_rate"].(float64); ok {
			proc.SuccessRate = rate
		}
		proc.Description = entry.Content
		procedures = append(procedures, proc)
	}

	return procedures, nil
}

// RecordOutcome updates a procedure's success rate based on execution outcome.
func (m *Manager) RecordOutcome(ctx context.Context, procedureID string, success bool) error {
	entry, err := m.provider.Get(ctx, procedureID)
	if err != nil {
		return fmt.Errorf("get procedure: %w", err)
	}

	usageCount := 1
	successRate := 1.0

	if count, ok := entry.Metadata["usage_count"].(float64); ok {
		usageCount = int(count) + 1
	}
	if rate, ok := entry.Metadata["success_rate"].(float64); ok {
		// Exponential moving average
		if success {
			successRate = rate*0.9 + 0.1
		} else {
			successRate = rate * 0.9
		}
	}

	entry.Metadata["usage_count"] = usageCount
	entry.Metadata["success_rate"] = successRate

	return m.provider.Update(ctx, entry)
}
