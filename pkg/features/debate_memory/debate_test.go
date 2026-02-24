package debate_memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is a minimal in-memory MemoryProvider for unit testing.
type testProvider struct {
	name      types.MemorySource
	entries   map[string]*types.MemoryEntry
	healthy   bool
	addErr    error
	searchErr error
}

func newTestProvider(name types.MemorySource) *testProvider {
	return &testProvider{
		name:    name,
		entries: make(map[string]*types.MemoryEntry),
		healthy: true,
	}
}

func (p *testProvider) Name() types.MemorySource { return p.name }

func (p *testProvider) Add(_ context.Context, entry *types.MemoryEntry) error {
	if p.addErr != nil {
		return p.addErr
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Search(
	_ context.Context, req *types.SearchRequest,
) (*types.SearchResult, error) {
	if p.searchErr != nil {
		return nil, p.searchErr
	}
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		entries = append(entries, e)
		if len(entries) >= req.TopK {
			break
		}
	}
	return &types.SearchResult{
		Entries:  entries,
		Total:    len(entries),
		Duration: 1 * time.Millisecond,
		Sources:  []types.MemorySource{p.name},
	}, nil
}

func (p *testProvider) Get(_ context.Context, id string) (*types.MemoryEntry, error) {
	if e, ok := p.entries[id]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found")
}

func (p *testProvider) Update(_ context.Context, entry *types.MemoryEntry) error {
	if _, ok := p.entries[entry.ID]; !ok {
		return fmt.Errorf("not found")
	}
	p.entries[entry.ID] = entry
	return nil
}

func (p *testProvider) Delete(_ context.Context, id string) error {
	delete(p.entries, id)
	return nil
}

func (p *testProvider) GetHistory(
	_ context.Context, _ string, limit int,
) ([]*types.MemoryEntry, error) {
	var entries []*types.MemoryEntry
	for _, e := range p.entries {
		entries = append(entries, e)
		if len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

func (p *testProvider) Health(_ context.Context) error {
	if !p.healthy {
		return fmt.Errorf("unhealthy")
	}
	return nil
}

func TestAugmenter_GetDebateContext(t *testing.T) {
	tests := []struct {
		name         string
		seedEntries  map[string]*types.MemoryEntry
		topic        string
		sessionID    string
		expectMinTot int
	}{
		{
			name: "returns context with relevant memories, decisions, patterns",
			seedEntries: map[string]*types.MemoryEntry{
				"m1": {
					ID:       "m1",
					Content:  "Go concurrency patterns",
					Type:     types.MemoryTypeFact,
					Metadata: map[string]interface{}{},
				},
				"m2": {
					ID:       "m2",
					Content:  "debate decision on architecture",
					Type:     types.MemoryTypeFact,
					Metadata: map[string]interface{}{},
				},
				"m3": {
					ID:       "m3",
					Content:  "pattern for error handling",
					Type:     types.MemoryTypeProcedural,
					Metadata: map[string]interface{}{},
				},
			},
			topic:        "concurrency patterns",
			sessionID:    "sess-1",
			expectMinTot: 1, // at least RelevantMemories populated
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			for k, v := range tc.seedEntries {
				prov.entries[k] = v
			}

			aug := NewAugmenter(prov)
			debateCtx, err := aug.GetDebateContext(
				context.Background(), tc.topic, tc.sessionID,
			)
			require.NoError(t, err)
			require.NotNil(t, debateCtx)

			assert.Equal(t, tc.sessionID, debateCtx.SessionID)
			assert.Equal(t, tc.topic, debateCtx.Topic)
			assert.NotEmpty(t, debateCtx.RelevantMemories)
			assert.NotNil(t, debateCtx.PastDecisions)
			assert.NotNil(t, debateCtx.Patterns)
			assert.GreaterOrEqual(t, debateCtx.TotalRetrieved, tc.expectMinTot)
			assert.Greater(t, debateCtx.RetrievalTime, time.Duration(0))
		})
	}
}

func TestAugmenter_GetDebateContext_SearchError(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.searchErr = fmt.Errorf("search backend unavailable")

	aug := NewAugmenter(prov)
	_, err := aug.GetDebateContext(
		context.Background(), "any topic", "sess-err",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search debate context")
}

func TestAugmenter_StoreDebateOutcome(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		topic      string
		consensus  string
		confidence float64
	}{
		{
			name:       "stores outcome with correct metadata",
			sessionID:  "sess-42",
			topic:      "best testing strategy",
			consensus:  "Use table-driven tests",
			confidence: 0.92,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			aug := NewAugmenter(prov)

			err := aug.StoreDebateOutcome(
				context.Background(),
				tc.sessionID, tc.topic, tc.consensus, tc.confidence,
			)
			require.NoError(t, err)
			require.Len(t, prov.entries, 1)

			// Get the stored entry
			var stored *types.MemoryEntry
			for _, e := range prov.entries {
				stored = e
			}

			assert.Contains(t, stored.Content, "[DEBATE_OUTCOME:")
			assert.Contains(t, stored.Content, tc.sessionID)
			assert.Equal(t, types.MemoryTypeFact, stored.Type)
			assert.Equal(t, tc.sessionID,
				stored.Metadata["debate_session_id"])
			assert.Equal(t, tc.topic, stored.Metadata["debate_topic"])
			assert.Equal(t, tc.consensus,
				stored.Metadata["debate_consensus"])
			assert.Equal(t, tc.confidence,
				stored.Metadata["debate_confidence"])
		})
	}
}

func TestAugmenter_StoreAgentInsight(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		agentID   string
		insight   string
	}{
		{
			name:      "stores insight with correct type and agent",
			sessionID: "sess-99",
			agentID:   "agent-alpha",
			insight:   "Consider using retry logic with backoff",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			aug := NewAugmenter(prov)

			err := aug.StoreAgentInsight(
				context.Background(),
				tc.sessionID, tc.agentID, tc.insight,
			)
			require.NoError(t, err)
			require.Len(t, prov.entries, 1)

			var stored *types.MemoryEntry
			for _, e := range prov.entries {
				stored = e
			}

			assert.Contains(t, stored.Content, "[DEBATE_INSIGHT:")
			assert.Contains(t, stored.Content, tc.sessionID)
			assert.Contains(t, stored.Content, tc.agentID)
			assert.Equal(t, types.MemoryTypeEpisodic, stored.Type)
			assert.Equal(t, tc.agentID, stored.AgentID)
			assert.Equal(t, tc.sessionID,
				stored.Metadata["debate_session_id"])
			assert.Equal(t, tc.agentID,
				stored.Metadata["agent_id"])
		})
	}
}

func TestAugmenter_ExtractKeywords(t *testing.T) {
	tests := []struct {
		name         string
		topic        string
		mustExclude  []string
		mustInclude  []string
	}{
		{
			name:  "stop words removed from topic",
			topic: "What is the best approach for testing",
			mustExclude: []string{
				"what", "is", "the", "for",
			},
			mustInclude: []string{
				"best", "approach", "testing",
			},
		},
		{
			name:         "short words under 3 chars excluded",
			topic:        "go is an ok language",
			mustExclude:  []string{"go", "is", "an", "ok"},
			mustInclude:  []string{"language"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractKeywords(tc.topic)
			words := strings.Fields(result)

			for _, excluded := range tc.mustExclude {
				assert.NotContains(t, words, excluded,
					"%q should be excluded", excluded)
			}
			for _, included := range tc.mustInclude {
				assert.Contains(t, words, included,
					"%q should be included", included)
			}
		})
	}
}
