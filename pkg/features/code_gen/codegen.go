// Package code_gen implements Memory-Driven Code Generation for HelixMemory.
// It leverages stored coding patterns, conventions, and project DNA
// to provide context-aware code generation assistance.
package code_gen

import (
	"context"
	"fmt"
	"strings"

	"digital.vasic.helixmemory/pkg/types"
)

// CodeContext provides code-relevant memory context for generation tasks.
type CodeContext struct {
	Patterns    []*types.MemoryEntry `json:"patterns"`
	Conventions []*types.MemoryEntry `json:"conventions"`
	Examples    []*types.MemoryEntry `json:"examples"`
	Related     []*types.MemoryEntry `json:"related"`
	Prompt      string               `json:"prompt"`
}

// Generator provides memory-enhanced code generation context.
type Generator struct {
	provider types.MemoryProvider
}

// NewGenerator creates a memory-driven code generation assistant.
func NewGenerator(provider types.MemoryProvider) *Generator {
	return &Generator{provider: provider}
}

// GetCodeContext retrieves code-relevant memory context.
func (g *Generator) GetCodeContext(ctx context.Context, task, language, project string) (*CodeContext, error) {
	codeCtx := &CodeContext{}

	// Fetch patterns for the language/project
	patternReq := &types.SearchRequest{
		Query: fmt.Sprintf("pattern %s %s", language, project),
		TopK:  10,
		Types: []types.MemoryType{types.MemoryTypeProcedural},
	}
	patternResult, err := g.provider.Search(ctx, patternReq)
	if err == nil {
		codeCtx.Patterns = patternResult.Entries
	}

	// Fetch conventions
	convReq := &types.SearchRequest{
		Query: fmt.Sprintf("convention style %s %s", language, project),
		TopK:  10,
	}
	convResult, err := g.provider.Search(ctx, convReq)
	if err == nil {
		codeCtx.Conventions = convResult.Entries
	}

	// Fetch related code examples
	exampleReq := &types.SearchRequest{
		Query: task,
		TopK:  5,
	}
	exampleResult, err := g.provider.Search(ctx, exampleReq)
	if err == nil {
		codeCtx.Examples = exampleResult.Entries
	}

	// Build augmented prompt
	codeCtx.Prompt = g.buildPrompt(task, language, project, codeCtx)

	return codeCtx, nil
}

// buildPrompt creates an augmented prompt from memory context.
func (g *Generator) buildPrompt(task, language, project string, codeCtx *CodeContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Task: %s\nLanguage: %s\nProject: %s\n\n", task, language, project))

	if len(codeCtx.Patterns) > 0 {
		sb.WriteString("Known Patterns:\n")
		for _, p := range codeCtx.Patterns {
			sb.WriteString(fmt.Sprintf("- %s\n", truncate(p.Content, 200)))
		}
		sb.WriteString("\n")
	}

	if len(codeCtx.Conventions) > 0 {
		sb.WriteString("Conventions:\n")
		for _, c := range codeCtx.Conventions {
			sb.WriteString(fmt.Sprintf("- %s\n", truncate(c.Content, 200)))
		}
		sb.WriteString("\n")
	}

	if len(codeCtx.Examples) > 0 {
		sb.WriteString("Related Examples:\n")
		for _, e := range codeCtx.Examples {
			sb.WriteString(fmt.Sprintf("- %s\n", truncate(e.Content, 300)))
		}
	}

	return sb.String()
}

// truncate limits a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
