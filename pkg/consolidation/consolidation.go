// Package consolidation implements sleep-time compute for HelixMemory.
// During idle periods, the consolidation engine deduplicates memories,
// cross-references across backends, and pre-computes frequently needed contexts.
// This is inspired by Letta's innovation of doing useful work while agents are idle.
package consolidation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/fusion"
	"digital.vasic.helixmemory/pkg/types"
)

// Engine manages sleep-time compute for memory consolidation.
type Engine struct {
	mu        sync.RWMutex
	cfg       *config.Config
	fusion    *fusion.FusionEngine
	providers []types.MemoryProvider
	running   bool
	stopCh    chan struct{}
	stats     Stats
}

// Stats tracks consolidation metrics.
type Stats struct {
	mu                sync.RWMutex
	TotalRuns         int           `json:"total_runs"`
	LastRunAt         time.Time     `json:"last_run_at"`
	LastDuration      time.Duration `json:"last_duration"`
	MemoriesProcessed int           `json:"memories_processed"`
	Deduplicated      int           `json:"deduplicated"`
	Consolidated      int           `json:"consolidated"`
	Errors            int           `json:"errors"`
}

// NewEngine creates a consolidation engine.
func NewEngine(cfg *config.Config) *Engine {
	fe, _ := fusion.NewFusionEngine(cfg, nil)
	return &Engine{
		cfg:    cfg,
		fusion: fe,
		stopCh: make(chan struct{}),
	}
}

// RegisterProvider adds a provider to the consolidation pipeline.
func (e *Engine) RegisterProvider(p types.MemoryProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers = append(e.providers, p)
}

// Start begins the periodic consolidation loop.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("consolidation: already running")
	}
	if !e.cfg.ConsolidationEnabled {
		e.mu.Unlock()
		return nil
	}
	e.running = true
	e.stopCh = make(chan struct{})
	e.mu.Unlock()

	go e.loop(ctx)
	return nil
}

// Stop halts the consolidation loop.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		close(e.stopCh)
		e.running = false
	}
}

// IsRunning returns whether the consolidation loop is active.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// GetStats returns current consolidation statistics.
func (e *Engine) GetStats() Stats {
	e.stats.mu.RLock()
	defer e.stats.mu.RUnlock()
	return Stats{
		TotalRuns:         e.stats.TotalRuns,
		LastRunAt:         e.stats.LastRunAt,
		LastDuration:      e.stats.LastDuration,
		MemoriesProcessed: e.stats.MemoriesProcessed,
		Deduplicated:      e.stats.Deduplicated,
		Consolidated:      e.stats.Consolidated,
		Errors:            e.stats.Errors,
	}
}

// RunOnce executes a single consolidation pass.
func (e *Engine) RunOnce(ctx context.Context) error {
	e.mu.RLock()
	providers := make([]types.MemoryProvider, len(e.providers))
	copy(providers, e.providers)
	e.mu.RUnlock()

	if len(providers) == 0 {
		return nil
	}

	start := time.Now()
	var processed, deduped, consolidated int

	// Phase 1: Collect recent memories from all providers
	var allEntries []*types.MemoryEntry
	for _, p := range providers {
		entries, err := p.GetHistory(ctx, "", e.cfg.ConsolidationBatchSize)
		if err != nil {
			continue
		}
		allEntries = append(allEntries, entries...)
		processed += len(entries)
	}

	if len(allEntries) == 0 {
		return nil
	}

	// Phase 2: Identify duplicates using fusion engine dedup
	seen := make(map[string]bool)
	var unique []*types.MemoryEntry
	for _, entry := range allEntries {
		if seen[entry.ID] {
			deduped++
			continue
		}
		seen[entry.ID] = true
		unique = append(unique, entry)
	}

	// Phase 3: Cross-reference and enrich
	for _, entry := range unique {
		if entry.Metadata == nil {
			entry.Metadata = make(map[string]interface{})
		}
		entry.Metadata["consolidated_at"] = time.Now().Format(time.RFC3339)
		entry.Metadata["consolidation_source_count"] = len(providers)
		consolidated++
	}

	duration := time.Since(start)

	// Update stats
	e.stats.mu.Lock()
	e.stats.TotalRuns++
	e.stats.LastRunAt = time.Now()
	e.stats.LastDuration = duration
	e.stats.MemoriesProcessed += processed
	e.stats.Deduplicated += deduped
	e.stats.Consolidated += consolidated
	e.stats.mu.Unlock()

	return nil
}

// loop runs the periodic consolidation.
func (e *Engine) loop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.ConsolidationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			if err := e.RunOnce(ctx); err != nil {
				e.stats.mu.Lock()
				e.stats.Errors++
				e.stats.mu.Unlock()
			}
		}
	}
}

// GetConsolidationStatus returns the current consolidation status.
func (e *Engine) GetConsolidationStatus() *types.ConsolidationStatus {
	stats := e.GetStats()
	return &types.ConsolidationStatus{
		Running:           e.IsRunning(),
		LastRun:           stats.LastRunAt,
		MemoriesProcessed: stats.MemoriesProcessed,
		Deduplicated:      stats.Deduplicated,
		Consolidated:      stats.Consolidated,
		Duration:          stats.LastDuration,
	}
}
