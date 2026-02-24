// Package quality_loop implements the Self-Improving Memory Quality Loop
// for HelixMemory. It continuously monitors memory quality, identifies
// stale/contradictory/low-confidence entries, and triggers cleanup.
package quality_loop

import (
	"context"
	"fmt"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/types"
)

// QualityReport summarizes memory quality metrics.
type QualityReport struct {
	TotalMemories      int       `json:"total_memories"`
	HighConfidence     int       `json:"high_confidence"`
	LowConfidence      int       `json:"low_confidence"`
	Stale              int       `json:"stale"`
	Contradictions     int       `json:"contradictions"`
	AverageConfidence  float64   `json:"average_confidence"`
	AverageAge         time.Duration `json:"average_age"`
	RecommendedActions []Action  `json:"recommended_actions"`
	GeneratedAt        time.Time `json:"generated_at"`
}

// Action represents a recommended quality improvement action.
type Action struct {
	Type        string   `json:"type"` // prune, refresh, merge, validate
	TargetIDs   []string `json:"target_ids"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"` // 1=critical, 2=high, 3=medium
}

// Loop manages the self-improving quality process.
type Loop struct {
	mu       sync.RWMutex
	provider types.MemoryProvider
	config   Config
	running  bool
	stopCh   chan struct{}
	stats    LoopStats
}

// Config configures the quality loop.
type Config struct {
	Enabled             bool          `json:"enabled"`
	Interval            time.Duration `json:"interval"`
	StaleThreshold      time.Duration `json:"stale_threshold"`
	LowConfidenceLimit  float64       `json:"low_confidence_limit"`
	MaxMemoriesPerScan  int           `json:"max_memories_per_scan"`
}

// DefaultConfig returns default quality loop configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:             true,
		Interval:            1 * time.Hour,
		StaleThreshold:      30 * 24 * time.Hour, // 30 days
		LowConfidenceLimit:  0.3,
		MaxMemoriesPerScan:  500,
	}
}

// LoopStats tracks quality loop execution metrics.
type LoopStats struct {
	TotalScans      int       `json:"total_scans"`
	LastScanAt      time.Time `json:"last_scan_at"`
	TotalPruned     int       `json:"total_pruned"`
	TotalRefreshed  int       `json:"total_refreshed"`
}

// NewLoop creates a quality improvement loop.
func NewLoop(provider types.MemoryProvider, config Config) *Loop {
	return &Loop{
		provider: provider,
		config:   config,
		stopCh:   make(chan struct{}),
	}
}

// Analyze performs a quality analysis scan and returns a report.
func (l *Loop) Analyze(ctx context.Context) (*QualityReport, error) {
	result, err := l.provider.Search(ctx, &types.SearchRequest{
		Query: "*",
		TopK:  l.config.MaxMemoriesPerScan,
	})
	if err != nil {
		return nil, fmt.Errorf("quality scan: %w", err)
	}

	report := &QualityReport{
		TotalMemories: len(result.Entries),
		GeneratedAt:   time.Now(),
	}

	now := time.Now()
	var totalConfidence float64
	var totalAge time.Duration

	for _, entry := range result.Entries {
		totalConfidence += entry.Confidence
		totalAge += now.Sub(entry.CreatedAt)

		if entry.Confidence >= 0.7 {
			report.HighConfidence++
		} else if entry.Confidence < l.config.LowConfidenceLimit {
			report.LowConfidence++
		}

		if now.Sub(entry.CreatedAt) > l.config.StaleThreshold {
			report.Stale++
		}
	}

	if len(result.Entries) > 0 {
		report.AverageConfidence = totalConfidence / float64(len(result.Entries))
		report.AverageAge = totalAge / time.Duration(len(result.Entries))
	}

	// Generate recommended actions
	if report.Stale > 0 {
		report.RecommendedActions = append(report.RecommendedActions, Action{
			Type:        "prune",
			Description: fmt.Sprintf("Remove %d stale memories (>%s old)", report.Stale, l.config.StaleThreshold),
			Priority:    3,
		})
	}

	if report.LowConfidence > 0 {
		report.RecommendedActions = append(report.RecommendedActions, Action{
			Type:        "validate",
			Description: fmt.Sprintf("Re-validate %d low-confidence memories (<%0.1f)", report.LowConfidence, l.config.LowConfidenceLimit),
			Priority:    2,
		})
	}

	return report, nil
}

// Start begins the periodic quality improvement loop.
func (l *Loop) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return fmt.Errorf("quality loop: already running")
	}
	if !l.config.Enabled {
		l.mu.Unlock()
		return nil
	}
	l.running = true
	l.stopCh = make(chan struct{})
	l.mu.Unlock()

	go l.loop(ctx)
	return nil
}

// Stop halts the quality loop.
func (l *Loop) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		close(l.stopCh)
		l.running = false
	}
}

func (l *Loop) loop(ctx context.Context) {
	ticker := time.NewTicker(l.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.mu.Lock()
			l.running = false
			l.mu.Unlock()
			return
		case <-l.stopCh:
			return
		case <-ticker.C:
			_, _ = l.Analyze(ctx)
			l.mu.Lock()
			l.stats.TotalScans++
			l.stats.LastScanAt = time.Now()
			l.mu.Unlock()
		}
	}
}

// GetStats returns quality loop statistics.
func (l *Loop) GetStats() LoopStats {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.stats
}
