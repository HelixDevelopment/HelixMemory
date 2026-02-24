package confidence

import (
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScorer_Score(t *testing.T) {
	scorer := NewScorer(nil)

	entry := &types.MemoryEntry{
		ID:          "test-1",
		Content:     "Go uses goroutines for concurrency",
		Source:      types.SourceLetta,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		AccessCount: 50,
	}

	score := scorer.Score(entry, 3)

	assert.GreaterOrEqual(t, score, 0.0, "score must be >= 0")
	assert.LessOrEqual(t, score, 1.0, "score must be <= 1")
	// With Letta source (0.95), 3 cross-validations (1.0), recent
	// creation, high access, and medium content, the score should
	// be reasonably high.
	assert.Greater(t, score, 0.5, "expected a high score for this entry")
}

func TestScorer_Score_DefaultWeights(t *testing.T) {
	scorer := NewScorer(nil)
	require.NotNil(t, scorer)
	require.NotNil(t, scorer.weights)

	assert.InDelta(t, 0.30, scorer.weights.SourceReliability, 0.001)
	assert.InDelta(t, 0.25, scorer.weights.CrossValidation, 0.001)
	assert.InDelta(t, 0.20, scorer.weights.RecencyDecay, 0.001)
	assert.InDelta(t, 0.15, scorer.weights.AccessFrequency, 0.001)
	assert.InDelta(t, 0.10, scorer.weights.ContentCoherence, 0.001)
}

func TestScorer_SourceReliabilityScore(t *testing.T) {
	tests := []struct {
		name     string
		source   types.MemorySource
		expected float64
	}{
		{"Letta", types.SourceLetta, 0.95},
		{"Mem0", types.SourceMem0, 0.85},
		{"Graphiti", types.SourceGraphiti, 0.85},
		{"Cognee", types.SourceCognee, 0.80},
		{"Fusion", types.SourceFusion, 0.90},
		{"unknown", types.MemorySource("unknown"), 0.50},
	}

	scorer := NewScorer(nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := scorer.sourceReliabilityScore(tc.source)
			assert.InDelta(t, tc.expected, score, 0.001)
		})
	}
}

func TestScorer_CrossValidationScore(t *testing.T) {
	tests := []struct {
		name        string
		validations int
		expected    float64
	}{
		{"zero validations", 0, 0.3},
		{"one validation", 1, 0.533},
		{"two validations", 2, 0.766},
		{"three or more validations", 3, 1.0},
		{"five validations", 5, 1.0},
	}

	scorer := NewScorer(nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := scorer.crossValidationScore(tc.validations)
			assert.InDelta(t, tc.expected, score, 0.01)
		})
	}
}

func TestScorer_RecencyDecayScore(t *testing.T) {
	scorer := NewScorer(nil)

	t.Run("very recent entry near 1.0", func(t *testing.T) {
		recent := time.Now().Add(-10 * time.Minute)
		score := scorer.recencyDecayScore(recent)
		assert.Greater(t, score, 0.95,
			"a 10-minute-old entry should score near 1.0")
	})

	t.Run("old entry significantly lower", func(t *testing.T) {
		old := time.Now().Add(-30 * 24 * time.Hour)
		score := scorer.recencyDecayScore(old)
		assert.Less(t, score, 0.10,
			"a 30-day-old entry should score significantly lower")
	})

	t.Run("decay is monotonic", func(t *testing.T) {
		recent := scorer.recencyDecayScore(time.Now())
		oneDay := scorer.recencyDecayScore(time.Now().Add(-24 * time.Hour))
		oneWeek := scorer.recencyDecayScore(time.Now().Add(-7 * 24 * time.Hour))
		assert.Greater(t, recent, oneDay)
		assert.Greater(t, oneDay, oneWeek)
	})
}

func TestScorer_AccessFrequencyScore(t *testing.T) {
	tests := []struct {
		name        string
		accessCount int
		expectMin   float64
		expectMax   float64
	}{
		{"zero accesses", 0, 0.1, 0.1},
		{"one access", 1, 0.1, 0.5},
		{"high accesses", 999, 0.9, 1.0},
	}

	scorer := NewScorer(nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := scorer.accessFrequencyScore(tc.accessCount)
			assert.GreaterOrEqual(t, score, tc.expectMin)
			assert.LessOrEqual(t, score, tc.expectMax)
		})
	}
}

func TestScorer_ContentCoherenceScore(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected float64
	}{
		{"empty content", "", 0.0},
		{"short content", "hi", 0.3},
		{"very long content", string(make([]byte, 5001)), 0.7},
		{"medium content", "This is a reasonably sized piece of content for testing.", 0.8},
	}

	scorer := NewScorer(nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := scorer.contentCoherenceScore(tc.content)
			assert.InDelta(t, tc.expected, score, 0.001)
		})
	}
}

func TestScorer_Score_Clamped(t *testing.T) {
	// Use extreme weights to try to push score beyond bounds.
	extremeWeights := &ScoreWeights{
		SourceReliability: 2.0,
		CrossValidation:   2.0,
		RecencyDecay:      2.0,
		AccessFrequency:   2.0,
		ContentCoherence:  2.0,
	}

	scorer := NewScorer(extremeWeights)

	entry := &types.MemoryEntry{
		ID:          "max-entry",
		Content:     "moderate length content that should score well",
		Source:      types.SourceLetta,
		CreatedAt:   time.Now(),
		AccessCount: 1000,
	}

	score := scorer.Score(entry, 5)
	assert.LessOrEqual(t, score, 1.0, "score must never exceed 1.0")
	assert.GreaterOrEqual(t, score, 0.0, "score must never go below 0.0")
}

func TestDefaultWeights(t *testing.T) {
	w := DefaultWeights()
	sum := w.SourceReliability + w.CrossValidation + w.RecencyDecay +
		w.AccessFrequency + w.ContentCoherence

	assert.InDelta(t, 1.0, sum, 0.001, "default weights must sum to 1.0")
}

func TestProvenance_New(t *testing.T) {
	entry := &types.MemoryEntry{
		ID:        "prov-1",
		Source:    types.SourceMem0,
		CreatedAt: time.Now().Add(-2 * time.Hour),
	}

	prov := NewProvenance(entry)

	assert.Equal(t, types.SourceMem0, prov.OriginalSource)
	assert.Equal(t, "prov-1", prov.OriginalID)
	assert.Equal(t, entry.CreatedAt, prov.CreatedAt)
	assert.Empty(t, prov.Transformations)
}

func TestProvenance_AddTransformation(t *testing.T) {
	entry := &types.MemoryEntry{
		ID:        "prov-2",
		Source:    types.SourceCognee,
		CreatedAt: time.Now(),
	}

	prov := NewProvenance(entry)

	prov.AddTransformation("fusion", "merged with mem0 entry")
	prov.AddTransformation("enrichment", "added temporal context")

	require.Len(t, prov.Transformations, 2)
	assert.Equal(t, "fusion", prov.Transformations[0].Type)
	assert.Equal(t, "merged with mem0 entry", prov.Transformations[0].Details)
	assert.Equal(t, "enrichment", prov.Transformations[1].Type)
	assert.Equal(t, "added temporal context", prov.Transformations[1].Details)
	assert.False(t, prov.Transformations[0].Timestamp.IsZero())
	assert.False(t, prov.Transformations[1].Timestamp.IsZero())
}
