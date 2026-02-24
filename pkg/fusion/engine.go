// Package fusion implements the 3-stage memory fusion engine for HelixMemory.
// Stage 1: Collection & Normalization — gather results from all backends.
// Stage 2: Deduplication — cosine similarity threshold (default 0.92).
// Stage 3: Cross-Source Re-Ranking — weighted scoring formula.
package fusion

import (
	"math"
	"sort"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"
)

// Engine performs multi-source memory fusion with deduplication and re-ranking.
type Engine struct {
	dedupThreshold  float64
	relevanceWeight float64
	recencyWeight   float64
	sourceWeight    float64
	typeWeight      float64
}

// NewEngine creates a fusion engine from configuration.
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		dedupThreshold:  cfg.FusionDedupThreshold,
		relevanceWeight: cfg.FusionRelevanceWeight,
		recencyWeight:   cfg.FusionRecencyWeight,
		sourceWeight:    cfg.FusionSourceWeight,
		typeWeight:      cfg.FusionTypeWeight,
	}
}

// Fuse takes results from multiple backends and produces a unified, deduplicated,
// re-ranked list of memory entries.
func (e *Engine) Fuse(results []*types.SearchResult, req *types.SearchRequest) *types.SearchResult {
	// Stage 1: Collection & Normalization
	all := e.collect(results)

	// Stage 2: Deduplication
	deduped := e.deduplicate(all)

	// Stage 3: Cross-Source Re-Ranking
	ranked := e.rerank(deduped, req)

	// Apply TopK limit
	if req.TopK > 0 && len(ranked) > req.TopK {
		ranked = ranked[:req.TopK]
	}

	// Collect all sources
	sourceSet := make(map[types.MemorySource]struct{})
	for _, r := range results {
		if r == nil {
			continue
		}
		for _, s := range r.Sources {
			sourceSet[s] = struct{}{}
		}
	}
	sources := make([]types.MemorySource, 0, len(sourceSet))
	for s := range sourceSet {
		sources = append(sources, s)
	}

	totalDuration := time.Duration(0)
	for _, r := range results {
		if r == nil {
			continue
		}
		totalDuration += r.Duration
	}

	return &types.SearchResult{
		Entries:  ranked,
		Total:    len(ranked),
		Duration: totalDuration,
		Sources:  sources,
	}
}

// collect gathers all entries from multiple search results and normalizes scores.
func (e *Engine) collect(results []*types.SearchResult) []*types.MemoryEntry {
	var all []*types.MemoryEntry
	for _, r := range results {
		if r == nil {
			continue
		}
		all = append(all, r.Entries...)
	}

	// Normalize relevance scores to [0, 1]
	if len(all) > 0 {
		maxScore := 0.0
		for _, entry := range all {
			if entry.Relevance > maxScore {
				maxScore = entry.Relevance
			}
		}
		if maxScore > 0 {
			for _, entry := range all {
				entry.Relevance = entry.Relevance / maxScore
			}
		}
	}

	return all
}

// deduplicate removes near-duplicate entries using content similarity.
func (e *Engine) deduplicate(entries []*types.MemoryEntry) []*types.MemoryEntry {
	if len(entries) <= 1 {
		return entries
	}

	// Use embedding-based similarity if available, otherwise content-based
	var result []*types.MemoryEntry
	seen := make(map[int]bool)

	for i, entry := range entries {
		if seen[i] {
			continue
		}

		// Check against all previously accepted entries
		isDuplicate := false
		for j := range result {
			sim := e.similarity(entry, result[j])
			if sim >= e.dedupThreshold {
				isDuplicate = true
				// Keep the one with higher confidence
				if entry.Confidence > result[j].Confidence {
					result[j] = entry
				}
				break
			}
		}

		if !isDuplicate {
			result = append(result, entry)
		}
		seen[i] = true
	}

	return result
}

// similarity computes similarity between two memory entries.
// Uses cosine similarity on embeddings if available, otherwise Jaccard on content.
func (e *Engine) similarity(a, b *types.MemoryEntry) float64 {
	// Prefer embedding-based similarity
	if len(a.Embedding) > 0 && len(b.Embedding) > 0 && len(a.Embedding) == len(b.Embedding) {
		return cosineSimilarity(a.Embedding, b.Embedding)
	}

	// Fall back to Jaccard similarity on content tokens
	return jaccardSimilarity(a.Content, b.Content)
}

// rerank scores and sorts entries using the weighted formula:
// score = relevance*0.40 + recency*0.25 + source*0.20 + type*0.15
func (e *Engine) rerank(entries []*types.MemoryEntry, req *types.SearchRequest) []*types.MemoryEntry {
	now := time.Now()

	for _, entry := range entries {
		relevanceScore := entry.Relevance
		recencyScore := e.recencyScore(entry.CreatedAt, now)
		sourceScore := e.sourceScore(entry.Source)
		typeScore := e.typeScore(entry.Type, req)

		entry.Relevance = relevanceScore*e.relevanceWeight +
			recencyScore*e.recencyWeight +
			sourceScore*e.sourceWeight +
			typeScore*e.typeWeight
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Relevance > entries[j].Relevance
	})

	return entries
}

// recencyScore computes a decay-based recency score.
// Returns 1.0 for very recent, decays exponentially over days.
func (e *Engine) recencyScore(created time.Time, now time.Time) float64 {
	hours := now.Sub(created).Hours()
	if hours < 0 {
		hours = 0
	}
	// Half-life of ~7 days (168 hours)
	return math.Exp(-0.00413 * hours)
}

// sourceScore returns a trust score for each backend.
func (e *Engine) sourceScore(source types.MemorySource) float64 {
	switch source {
	case types.SourceLetta:
		return 0.95 // Stateful agent, highest trust
	case types.SourceMem0:
		return 0.85 // Dynamic extraction, well-validated
	case types.SourceCognee:
		return 0.80 // Knowledge graph, good for relations
	case types.SourceGraphiti:
		return 0.85 // Temporal awareness, high trust
	case types.SourceFusion:
		return 0.90 // Already fused, high quality
	default:
		return 0.50
	}
}

// typeScore returns relevance for a memory type given the query context.
func (e *Engine) typeScore(memType types.MemoryType, req *types.SearchRequest) float64 {
	// If specific types were requested, boost matching types
	if len(req.Types) > 0 {
		for _, t := range req.Types {
			if t == memType {
				return 1.0
			}
		}
		return 0.3
	}

	// Default type scores
	switch memType {
	case types.MemoryTypeFact:
		return 0.85
	case types.MemoryTypeCore:
		return 0.90
	case types.MemoryTypeGraph:
		return 0.80
	case types.MemoryTypeTemporal:
		return 0.75
	case types.MemoryTypeEpisodic:
		return 0.70
	case types.MemoryTypeProcedural:
		return 0.85
	default:
		return 0.50
	}
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// jaccardSimilarity computes Jaccard similarity between two strings
// using word-level tokenization.
func jaccardSimilarity(a, b string) float64 {
	tokensA := tokenize(a)
	tokensB := tokenize(b)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}

	intersection := 0
	setB := make(map[string]struct{})
	for _, t := range tokensB {
		setB[t] = struct{}{}
	}

	setA := make(map[string]struct{})
	for _, t := range tokensA {
		setA[t] = struct{}{}
		if _, ok := setB[t]; ok {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

// tokenize splits a string into lowercase word tokens.
func tokenize(s string) []string {
	var tokens []string
	var current []byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32 // toLower
		}
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			current = append(current, c)
		} else if len(current) > 0 {
			tokens = append(tokens, string(current))
			current = current[:0]
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}

	return tokens
}
