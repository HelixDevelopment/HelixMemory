// Package confidence implements Confidence Scoring & Provenance for HelixMemory.
// Every memory entry gets a confidence score based on source reliability,
// cross-validation, age, and access patterns. Full provenance tracking
// ensures every memory can be traced back to its origin.
package confidence

import (
	"math"
	"time"

	"digital.vasic.helixmemory/pkg/types"
)

// ScoreWeights configures the confidence scoring formula.
type ScoreWeights struct {
	SourceReliability   float64 `json:"source_reliability"`
	CrossValidation     float64 `json:"cross_validation"`
	RecencyDecay        float64 `json:"recency_decay"`
	AccessFrequency     float64 `json:"access_frequency"`
	ContentCoherence    float64 `json:"content_coherence"`
}

// DefaultWeights returns the default confidence scoring weights.
func DefaultWeights() *ScoreWeights {
	return &ScoreWeights{
		SourceReliability: 0.30,
		CrossValidation:   0.25,
		RecencyDecay:      0.20,
		AccessFrequency:   0.15,
		ContentCoherence:  0.10,
	}
}

// Scorer calculates confidence scores for memory entries.
type Scorer struct {
	weights *ScoreWeights
}

// NewScorer creates a confidence scorer.
func NewScorer(weights *ScoreWeights) *Scorer {
	if weights == nil {
		weights = DefaultWeights()
	}
	return &Scorer{weights: weights}
}

// Score calculates the confidence score for a memory entry.
func (s *Scorer) Score(entry *types.MemoryEntry, crossValidations int) float64 {
	sourceScore := s.sourceReliabilityScore(entry.Source)
	crossValScore := s.crossValidationScore(crossValidations)
	recencyScore := s.recencyDecayScore(entry.CreatedAt)
	accessScore := s.accessFrequencyScore(entry.AccessCount)
	coherenceScore := s.contentCoherenceScore(entry.Content)

	score := sourceScore*s.weights.SourceReliability +
		crossValScore*s.weights.CrossValidation +
		recencyScore*s.weights.RecencyDecay +
		accessScore*s.weights.AccessFrequency +
		coherenceScore*s.weights.ContentCoherence

	// Clamp to [0, 1]
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// sourceReliabilityScore rates the source's inherent trustworthiness.
func (s *Scorer) sourceReliabilityScore(source types.MemorySource) float64 {
	switch source {
	case types.SourceLetta:
		return 0.95 // Stateful agent, highest reliability
	case types.SourceMem0:
		return 0.85 // Well-validated extraction
	case types.SourceGraphiti:
		return 0.85 // Temporal awareness adds trust
	case types.SourceCognee:
		return 0.80 // Graph-based, good for relations
	case types.SourceFusion:
		return 0.90 // Already cross-validated
	default:
		return 0.50
	}
}

// crossValidationScore rewards memories confirmed by multiple sources.
func (s *Scorer) crossValidationScore(validations int) float64 {
	if validations <= 0 {
		return 0.3
	}
	if validations >= 3 {
		return 1.0
	}
	return 0.3 + float64(validations)*0.233
}

// recencyDecayScore applies exponential decay based on age.
func (s *Scorer) recencyDecayScore(createdAt time.Time) float64 {
	hours := time.Since(createdAt).Hours()
	if hours < 0 {
		hours = 0
	}
	// Half-life of ~7 days
	return math.Exp(-0.00413 * hours)
}

// accessFrequencyScore rewards frequently accessed memories.
func (s *Scorer) accessFrequencyScore(accessCount int) float64 {
	if accessCount <= 0 {
		return 0.1
	}
	// Logarithmic scaling
	score := math.Log10(float64(accessCount)+1) / 3.0
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// contentCoherenceScore rates content quality heuristically.
func (s *Scorer) contentCoherenceScore(content string) float64 {
	if len(content) == 0 {
		return 0.0
	}
	if len(content) < 10 {
		return 0.3
	}
	if len(content) > 5000 {
		return 0.7 // Very long content may be less focused
	}
	return 0.8
}

// Provenance tracks the origin and transformation history of a memory.
type Provenance struct {
	OriginalSource types.MemorySource `json:"original_source"`
	OriginalID     string             `json:"original_id"`
	CreatedAt      time.Time          `json:"created_at"`
	Transformations []Transformation  `json:"transformations,omitempty"`
}

// Transformation records a change applied to a memory.
type Transformation struct {
	Type      string    `json:"type"` // fusion, consolidation, enrichment
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details,omitempty"`
}

// NewProvenance creates a provenance record for a memory entry.
func NewProvenance(entry *types.MemoryEntry) *Provenance {
	return &Provenance{
		OriginalSource: entry.Source,
		OriginalID:     entry.ID,
		CreatedAt:      entry.CreatedAt,
	}
}

// AddTransformation records a transformation in the provenance chain.
func (p *Provenance) AddTransformation(transformType, details string) {
	p.Transformations = append(p.Transformations, Transformation{
		Type:      transformType,
		Timestamp: time.Now(),
		Details:   details,
	})
}
