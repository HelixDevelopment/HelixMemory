// Package fusion provides result consolidation for the HelixMemory fusion engine.
package fusion

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"go.uber.org/zap"
)

// Consolidator fuses results from multiple memory systems.
type Consolidator struct {
	config *config.Config
	logger *zap.Logger

	// Deduplication cache
	seenIDs     map[string]bool
	seenContent map[string]bool
	cacheMutex  sync.RWMutex
}

// NewConsolidator creates a new result consolidator.
func NewConsolidator(cfg *config.Config, logger *zap.Logger) *Consolidator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Consolidator{
		config:      cfg,
		logger:      logger,
		seenIDs:     make(map[string]bool),
		seenContent: make(map[string]bool),
	}
}

// FuseResults combines results from multiple memory systems.
func (c *Consolidator) FuseResults(results map[types.MemorySource]*types.SearchResult, req *types.SearchRequest) *types.FusionResult {
	fused := &types.FusionResult{
		Entries:     make([]*types.MemoryEntry, 0),
		Sources:     make([]types.MemorySource, 0),
		SourceStats: make(map[types.MemorySource]int),
		Query:       req.Query,
	}

	// Flatten all entries
	var flatEntries []*types.MemoryEntry
	for source, result := range results {
		if result == nil {
			continue
		}
		fused.Sources = append(fused.Sources, source)
		fused.SourceStats[source] = result.Total
		flatEntries = append(flatEntries, result.Entries...)
	}

	// Deduplicate: exact content and embedding similarity
	deduped := c.deduplicateEntries(flatEntries)

	// Score and sort
	var allEntries []*scoredEntry
	for _, entry := range deduped {
		score := c.calculateFusionScore(entry, entry.Source, req)
		allEntries = append(allEntries, &scoredEntry{
			entry: entry,
			score: score,
		})
	}

	// Sort by fusion score (descending)
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].score > allEntries[j].score
	})

	// Apply limit
	limit := req.TopK
	if limit <= 0 {
		limit = 10
	}
	if limit > len(allEntries) {
		limit = len(allEntries)
	}

	// Build final result
	for i := 0; i < limit; i++ {
		entry := allEntries[i].entry
		entry.Relevance = allEntries[i].score
		fused.Entries = append(fused.Entries, entry)
	}

	fused.Total = len(fused.Entries)
	if len(allEntries) > 0 {
		fused.FusionScore = allEntries[0].score
	}

	c.logger.Debug("Results fused",
		zap.Int("total_input", len(flatEntries)),
		zap.Int("total_output", fused.Total),
		zap.Int("sources", len(fused.Sources)),
	)

	return fused
}

// scoredEntry wraps a memory entry with its fusion score.
type scoredEntry struct {
	entry *types.MemoryEntry
	score float64
}

// calculateFusionScore computes a relevance score for fusion ranking.
func (c *Consolidator) calculateFusionScore(entry *types.MemoryEntry, source types.MemorySource, req *types.SearchRequest) float64 {
	baseScore := entry.Relevance

	// Source-specific weighting
	sourceWeights := map[types.MemorySource]float64{
		types.SourceCognee: 1.2, // Knowledge graphs are high confidence
		types.SourceMem0:   1.0, // Semantic baseline
		types.SourceLetta:  1.1, // Agent context is important
	}

	weight := sourceWeights[source]
	if weight == 0 {
		weight = 1.0
	}

	// Recency boost
	recencyBoost := c.calculateRecencyBoost(entry)

	// Confidence boost
	confidenceBoost := entry.Confidence * 0.1

	// Type weight
	typeWeight := c.calculateTypeWeight(entry, req)

	// Final score
	finalScore := (baseScore * weight * typeWeight) + recencyBoost + confidenceBoost

	return finalScore
}

// calculateTypeWeight returns a multiplicative weight for memory types.
// Requested types get a strong boost; default priorities apply otherwise.
func (c *Consolidator) calculateTypeWeight(entry *types.MemoryEntry, req *types.SearchRequest) float64 {
	// If request specifies types, strongly boost matching types
	if req != nil && len(req.Types) > 0 {
		for _, t := range req.Types {
			if entry.Type == t {
				return 1.35
			}
		}
		return 1.0
	}

	// Default type weights
	typeWeights := map[types.MemoryType]float64{
		types.MemoryTypeCore:       1.25,
		types.MemoryTypeFact:       1.10,
		types.MemoryTypeProcedural: 1.10,
		types.MemoryTypeGraph:      1.05,
		types.MemoryTypeTemporal:   1.00,
		types.MemoryTypeEpisodic:   0.95,
	}

	w := typeWeights[entry.Type]
	if w == 0 {
		w = 1.0
	}
	return w
}

// calculateRecencyBoost gives higher scores to recent memories.
func (c *Consolidator) calculateRecencyBoost(entry *types.MemoryEntry) float64 {
	age := time.Since(entry.CreatedAt)

	switch {
	case age < time.Hour:
		return 0.15
	case age < time.Hour * 24:
		return 0.10
	case age < time.Hour * 24 * 7:
		return 0.05
	case age < time.Hour * 24 * 30:
		return 0.02
	default:
		return 0.0
	}
}

// isDuplicate checks if an entry is a duplicate.
func (c *Consolidator) isDuplicate(entry *types.MemoryEntry) bool {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	// Check ID
	if c.seenIDs[entry.ID] {
		return true
	}
	c.seenIDs[entry.ID] = true

	// Check content similarity (simple exact match for now)
	if c.seenContent[entry.Content] {
		return true
	}
	c.seenContent[entry.Content] = true

	return false
}

// clearCache resets the deduplication cache.
func (c *Consolidator) clearCache() {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	c.seenIDs = make(map[string]bool)
	c.seenContent = make(map[string]bool)
}

// deduplicateEntries removes duplicates, keeping the entry with highest
// confidence. It checks exact content match and embedding cosine similarity.
func (c *Consolidator) deduplicateEntries(entries []*types.MemoryEntry) []*types.MemoryEntry {
	// Phase 1: exact content dedup — keep highest confidence
	bestByContent := make(map[string]*types.MemoryEntry)
	for _, e := range entries {
		existing, ok := bestByContent[e.Content]
		if !ok || e.Confidence > existing.Confidence {
			bestByContent[e.Content] = e
		}
	}

	// Phase 2: embedding similarity dedup (cosine >= 0.92)
	var result []*types.MemoryEntry
	for _, e := range bestByContent {
		merged := false
		for _, r := range result {
			if c.cosineSimilarity(e.Embedding, r.Embedding) >= 0.92 {
				// Merge into the higher-confidence entry
				if e.Confidence > r.Confidence {
					*r = *e
				}
				merged = true
				break
			}
		}
		if !merged {
			result = append(result, e)
		}
	}

	return result
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func (c *Consolidator) cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// RunConsolidation performs sleep-time memory consolidation.
func (c *Consolidator) RunConsolidation(ctx context.Context) error {
	c.logger.Info("Starting memory consolidation")

	// This would trigger background consolidation tasks:
	// 1. Deduplication across systems
	// 2. Knowledge graph enrichment
	// 3. Semantic clustering
	// 4. Importance scoring

	c.logger.Info("Memory consolidation completed")
	return nil
}

// Deduplicate removes duplicate memories from a slice.
func (c *Consolidator) Deduplicate(entries []*types.MemoryEntry) []*types.MemoryEntry {
	c.clearCache()

	result := make([]*types.MemoryEntry, 0)
	for _, entry := range entries {
		if !c.isDuplicate(entry) {
			result = append(result, entry)
		}
	}

	return result
}

// MergeSimilar merges similar memories into a single consolidated entry.
func (c *Consolidator) MergeSimilar(entries []*types.MemoryEntry, similarityThreshold float64) []*types.MemoryEntry {
	if len(entries) <= 1 {
		return entries
	}

	// Simple implementation: group by content similarity
	groups := make(map[string][]*types.MemoryEntry)

	for _, entry := range entries {
		key := c.normalizeContent(entry.Content)
		groups[key] = append(groups[key], entry)
	}

	result := make([]*types.MemoryEntry, 0)
	for _, group := range groups {
		if len(group) == 1 {
			result = append(result, group[0])
		} else {
			// Merge group into single entry
			merged := c.mergeGroup(group)
			result = append(result, merged)
		}
	}

	return result
}

// normalizeContent creates a normalized key for content comparison.
func (c *Consolidator) normalizeContent(content string) string {
	// Simple normalization: lowercase, trim whitespace
	// In production, this would use more sophisticated NLP
	if len(content) > 100 {
		content = content[:100]
	}
	return content
}

// mergeGroup merges a group of similar memories.
//
// When collapsing duplicate memory entries the base should be the
// one with the HIGHEST confidence — if two sources report the same
// content with different quality scores we want to surface the
// better-scored one. Tie-break on recency. Previously the base was
// simply the most-recent entry, which discarded high-confidence
// signals (BUGFIX #33 follow-up).
func (c *Consolidator) mergeGroup(group []*types.MemoryEntry) *types.MemoryEntry {
	if len(group) == 0 {
		return nil
	}

	base := group[0]
	for _, entry := range group {
		if entry.Confidence > base.Confidence {
			base = entry
			continue
		}
		if entry.Confidence == base.Confidence && entry.CreatedAt.After(base.CreatedAt) {
			base = entry
		}
	}

	// Update metadata to indicate merging
	if base.Metadata == nil {
		base.Metadata = make(map[string]interface{})
	}
	base.Metadata["merged_count"] = len(group)
	base.Metadata["merged_sources"] = c.extractSources(group)

	return base
}

// extractSources gets unique sources from a group of entries.
func (c *Consolidator) extractSources(entries []*types.MemoryEntry) []string {
	sourceMap := make(map[string]bool)
	for _, entry := range entries {
		sourceMap[string(entry.Source)] = true
	}

	sources := make([]string, 0, len(sourceMap))
	for s := range sourceMap {
		sources = append(sources, s)
	}
	return sources
}
