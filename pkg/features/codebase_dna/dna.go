// Package codebase_dna implements Codebase DNA Profiling for HelixMemory.
// It builds a memory profile of coding patterns, preferences, architecture
// decisions, and tech stack details from analyzing codebase interactions.
package codebase_dna

import (
	"context"
	"fmt"
	"strings"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// Profile represents a codebase's DNA — its patterns, preferences, and conventions.
type Profile struct {
	ID              string                 `json:"id"`
	ProjectName     string                 `json:"project_name"`
	Language        string                 `json:"language"`
	Framework       string                 `json:"framework,omitempty"`
	Patterns        []Pattern              `json:"patterns"`
	Preferences     []Preference           `json:"preferences"`
	Conventions     []Convention           `json:"conventions"`
	Dependencies    []string               `json:"dependencies,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	LastAnalyzed    time.Time              `json:"last_analyzed"`
	ConfidenceScore float64                `json:"confidence_score"`
}

// Pattern represents a detected coding pattern.
type Pattern struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Frequency   int     `json:"frequency"`
	Confidence  float64 `json:"confidence"`
	Examples    []string `json:"examples,omitempty"`
}

// Preference represents a detected coding preference.
type Preference struct {
	Category string `json:"category"` // naming, style, architecture, testing
	Key      string `json:"key"`
	Value    string `json:"value"`
}

// Convention represents a project convention.
type Convention struct {
	Rule        string `json:"rule"`
	Scope       string `json:"scope"` // file, package, module, project
	Enforcement string `json:"enforcement"` // strict, recommended, optional
}

// Profiler analyzes code interactions and builds DNA profiles.
type Profiler struct {
	provider types.MemoryProvider
}

// NewProfiler creates a codebase DNA profiler.
func NewProfiler(provider types.MemoryProvider) *Profiler {
	return &Profiler{provider: provider}
}

// AnalyzeCode extracts patterns from code content and stores as memories.
func (p *Profiler) AnalyzeCode(ctx context.Context, code, language, projectName string) (*Profile, error) {
	profile := &Profile{
		ID:           uuid.New().String(),
		ProjectName:  projectName,
		Language:     language,
		Patterns:     p.detectPatterns(code, language),
		Preferences:  p.detectPreferences(code, language),
		Conventions:  p.detectConventions(code, language),
		LastAnalyzed: time.Now(),
	}

	// Calculate confidence based on code size and pattern count
	profile.ConfidenceScore = p.calculateConfidence(code, profile)

	// Store as memory entries
	for _, pattern := range profile.Patterns {
		entry := &types.MemoryEntry{
			ID:      uuid.New().String(),
			Content: fmt.Sprintf("[DNA:%s] Pattern: %s — %s", projectName, pattern.Name, pattern.Description),
			Type:    types.MemoryTypeProcedural,
			Source:  types.SourceFusion,
			Metadata: map[string]interface{}{
				"dna_project":  projectName,
				"dna_language": language,
				"dna_type":     "pattern",
				"pattern_name": pattern.Name,
				"frequency":    pattern.Frequency,
			},
			Confidence: pattern.Confidence,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := p.provider.Add(ctx, entry); err != nil {
			return nil, fmt.Errorf("store pattern: %w", err)
		}
	}

	return profile, nil
}

// GetProfile retrieves the DNA profile for a project.
func (p *Profiler) GetProfile(ctx context.Context, projectName string) (*Profile, error) {
	req := &types.SearchRequest{
		Query: fmt.Sprintf("[DNA:%s]", projectName),
		TopK:  100,
		Filter: map[string]interface{}{
			"dna_project": projectName,
		},
	}

	result, err := p.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search profile: %w", err)
	}

	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("no profile found for %s", projectName)
	}

	profile := &Profile{
		ProjectName:  projectName,
		LastAnalyzed: time.Now(),
	}

	for _, entry := range result.Entries {
		if lang, ok := entry.Metadata["dna_language"].(string); ok && profile.Language == "" {
			profile.Language = lang
		}
		if dnaType, ok := entry.Metadata["dna_type"].(string); ok {
			switch dnaType {
			case "pattern":
				name, _ := entry.Metadata["pattern_name"].(string)
				profile.Patterns = append(profile.Patterns, Pattern{
					Name:       name,
					Confidence: entry.Confidence,
				})
			}
		}
	}

	return profile, nil
}

func (p *Profiler) detectPatterns(code, language string) []Pattern {
	var patterns []Pattern

	switch language {
	case "go", "golang":
		if strings.Contains(code, "interface {") || strings.Contains(code, "interface{") {
			patterns = append(patterns, Pattern{
				Name:        "interface_abstraction",
				Description: "Uses Go interfaces for abstraction",
				Frequency:   strings.Count(code, "interface"),
				Confidence:  0.9,
			})
		}
		if strings.Contains(code, "func Test") {
			patterns = append(patterns, Pattern{
				Name:        "table_driven_tests",
				Description: "Uses Go testing patterns",
				Frequency:   strings.Count(code, "func Test"),
				Confidence:  0.85,
			})
		}
		if strings.Contains(code, "context.Context") {
			patterns = append(patterns, Pattern{
				Name:        "context_propagation",
				Description: "Propagates context through call chain",
				Frequency:   strings.Count(code, "context.Context"),
				Confidence:  0.95,
			})
		}
		if strings.Contains(code, "sync.Mutex") || strings.Contains(code, "sync.RWMutex") {
			patterns = append(patterns, Pattern{
				Name:        "mutex_concurrency",
				Description: "Uses mutexes for concurrent access control",
				Frequency:   strings.Count(code, "sync."),
				Confidence:  0.9,
			})
		}
		if strings.Contains(code, "fmt.Errorf") && strings.Contains(code, "%w") {
			patterns = append(patterns, Pattern{
				Name:        "error_wrapping",
				Description: "Wraps errors with context using %w",
				Frequency:   strings.Count(code, "%w"),
				Confidence:  0.9,
			})
		}
	case "python":
		if strings.Contains(code, "class ") && strings.Contains(code, "ABC") {
			patterns = append(patterns, Pattern{
				Name:        "abstract_base_classes",
				Description: "Uses ABCs for interface definition",
				Frequency:   1,
				Confidence:  0.85,
			})
		}
	}

	return patterns
}

func (p *Profiler) detectPreferences(code, language string) []Preference {
	var prefs []Preference

	if language == "go" || language == "golang" {
		if strings.Contains(code, "testify") {
			prefs = append(prefs, Preference{
				Category: "testing",
				Key:      "test_framework",
				Value:    "testify",
			})
		}
		if strings.Contains(code, "camelCase") || !strings.Contains(code, "snake_case") {
			prefs = append(prefs, Preference{
				Category: "naming",
				Key:      "convention",
				Value:    "camelCase",
			})
		}
	}

	return prefs
}

func (p *Profiler) detectConventions(code, language string) []Convention {
	var convs []Convention

	lines := strings.Split(code, "\n")
	maxLineLen := 0
	for _, line := range lines {
		if len(line) > maxLineLen {
			maxLineLen = len(line)
		}
	}

	if maxLineLen <= 100 {
		convs = append(convs, Convention{
			Rule:        "Line length <= 100 characters",
			Scope:       "file",
			Enforcement: "recommended",
		})
	}

	return convs
}

func (p *Profiler) calculateConfidence(code string, profile *Profile) float64 {
	lines := strings.Count(code, "\n")
	patternCount := len(profile.Patterns)

	lineScore := float64(lines) / 1000.0
	if lineScore > 1.0 {
		lineScore = 1.0
	}

	patternScore := float64(patternCount) / 10.0
	if patternScore > 1.0 {
		patternScore = 1.0
	}

	return lineScore*0.4 + patternScore*0.6
}
