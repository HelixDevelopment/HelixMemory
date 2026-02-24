// Package debate_memory implements Memory-Augmented AI Debate for HelixMemory.
// It provides memory-backed context injection for debate agents, enabling
// debates to leverage historical knowledge, past decisions, and learned patterns.
package debate_memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// DebateContext represents memory context for a debate session.
type DebateContext struct {
	SessionID       string               `json:"session_id"`
	Topic           string               `json:"topic"`
	RelevantMemories []*types.MemoryEntry `json:"relevant_memories"`
	PastDecisions   []*types.MemoryEntry `json:"past_decisions"`
	Patterns        []*types.MemoryEntry `json:"patterns"`
	TotalRetrieved  int                  `json:"total_retrieved"`
	RetrievalTime   time.Duration        `json:"retrieval_time"`
}

// Augmenter provides memory augmentation for debate sessions.
type Augmenter struct {
	provider types.MemoryProvider
}

// NewAugmenter creates a debate memory augmenter.
func NewAugmenter(provider types.MemoryProvider) *Augmenter {
	return &Augmenter{provider: provider}
}

// GetDebateContext retrieves relevant memory context for a debate topic.
func (a *Augmenter) GetDebateContext(ctx context.Context, topic string, sessionID string) (*DebateContext, error) {
	start := time.Now()

	debateCtx := &DebateContext{
		SessionID: sessionID,
		Topic:     topic,
	}

	// Search for relevant memories
	req := &types.SearchRequest{
		Query: topic,
		TopK:  20,
	}
	result, err := a.provider.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search debate context: %w", err)
	}

	debateCtx.RelevantMemories = result.Entries

	// Search for past debate decisions
	decisionReq := &types.SearchRequest{
		Query: fmt.Sprintf("debate decision %s", topic),
		TopK:  10,
		Types: []types.MemoryType{types.MemoryTypeFact},
	}
	decisionResult, err := a.provider.Search(ctx, decisionReq)
	if err == nil {
		debateCtx.PastDecisions = decisionResult.Entries
	}

	// Search for patterns
	patternReq := &types.SearchRequest{
		Query: fmt.Sprintf("pattern %s", extractKeywords(topic)),
		TopK:  10,
		Types: []types.MemoryType{types.MemoryTypeProcedural},
	}
	patternResult, err := a.provider.Search(ctx, patternReq)
	if err == nil {
		debateCtx.Patterns = patternResult.Entries
	}

	debateCtx.TotalRetrieved = len(debateCtx.RelevantMemories) +
		len(debateCtx.PastDecisions) + len(debateCtx.Patterns)
	debateCtx.RetrievalTime = time.Since(start)

	return debateCtx, nil
}

// StoreDebateOutcome saves the result of a debate as memory.
func (a *Augmenter) StoreDebateOutcome(ctx context.Context, sessionID, topic, consensus string, confidence float64) error {
	entry := &types.MemoryEntry{
		ID:      uuid.New().String(),
		Content: fmt.Sprintf("[DEBATE_OUTCOME:%s] Topic: %s | Consensus: %s", sessionID, topic, consensus),
		Type:    types.MemoryTypeFact,
		Source:  types.SourceFusion,
		Metadata: map[string]interface{}{
			"debate_session_id": sessionID,
			"debate_topic":      topic,
			"debate_consensus":  consensus,
			"debate_confidence": confidence,
		},
		Confidence: confidence,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	return a.provider.Add(ctx, entry)
}

// StoreAgentInsight saves an individual agent's insight from a debate.
func (a *Augmenter) StoreAgentInsight(ctx context.Context, sessionID, agentID, insight string) error {
	entry := &types.MemoryEntry{
		ID:      uuid.New().String(),
		Content: fmt.Sprintf("[DEBATE_INSIGHT:%s/%s] %s", sessionID, agentID, insight),
		Type:    types.MemoryTypeEpisodic,
		Source:  types.SourceFusion,
		Metadata: map[string]interface{}{
			"debate_session_id": sessionID,
			"agent_id":          agentID,
		},
		AgentID:   agentID,
		Confidence: 0.75,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return a.provider.Add(ctx, entry)
}

// extractKeywords extracts key terms from a topic string.
func extractKeywords(topic string) string {
	// Simple keyword extraction — remove common words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "can": true, "for": true, "of": true,
		"to": true, "in": true, "on": true, "at": true, "by": true,
		"with": true, "from": true, "and": true, "or": true, "not": true,
		"what": true, "which": true, "that": true, "this": true, "how": true,
	}

	words := strings.Fields(strings.ToLower(topic))
	var keywords []string
	for _, w := range words {
		if !stopWords[w] && len(w) > 2 {
			keywords = append(keywords, w)
		}
	}
	return strings.Join(keywords, " ")
}
