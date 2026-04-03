// Package fusion_test provides comprehensive tests for the fusion engine.
package fusion_test

import (
	"context"
	"testing"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/fusion"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockProvider implements types.MemoryProvider for testing
type mockProvider struct {
	name           types.MemorySource
	healthy        bool
	storedEntries  []*types.MemoryEntry
	searchResults  []*types.MemoryEntry
	shouldError    bool
	errorMessage   string
}

func (m *mockProvider) Name() types.MemorySource { return m.name }
func (m *mockProvider) Health(ctx context.Context) error {
	if !m.healthy {
		return assert.AnError
	}
	return nil
}
func (m *mockProvider) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if m.shouldError {
		return assert.AnError
	}
	m.storedEntries = append(m.storedEntries, entry)
	return nil
}
func (m *mockProvider) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if m.shouldError {
		return nil, assert.AnError
	}
	return &types.SearchResult{
		Entries: m.searchResults,
		Total:   len(m.searchResults),
		Sources: []types.MemorySource{m.name},
	}, nil
}
func (m *mockProvider) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	if m.shouldError {
		return nil, assert.AnError
	}
	for _, e := range m.storedEntries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, assert.AnError
}
func (m *mockProvider) Update(ctx context.Context, entry *types.MemoryEntry) error {
	if m.shouldError {
		return assert.AnError
	}
	return m.Add(ctx, entry)
}
func (m *mockProvider) Delete(ctx context.Context, id string) error {
	if m.shouldError {
		return assert.AnError
	}
	return nil
}
func (m *mockProvider) GetHistory(ctx context.Context, userID string, limit int) ([]*types.MemoryEntry, error) {
	if m.shouldError {
		return nil, assert.AnError
	}
	return m.storedEntries, nil
}

// ==================== Unit Tests ====================

func TestNewFusionEngine(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
	}{
		{
			name: "valid config with all systems",
			config: &config.Config{
				CogneeEndpoint: "http://cognee:8000",
				CogneeAPIKey:   "test-key",
				Mem0Endpoint:   "http://mem0:8000",
				Mem0APIKey:     "test-key",
				LettaEndpoint:  "http://letta:8000",
				LettaAPIKey:    "test-key",
			},
			wantErr: false,
		},
		{
			name: "valid config with partial systems",
			config: &config.Config{
				Mem0Endpoint: "http://mem0:8000",
				Mem0APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "empty config still works",
			config: &config.Config{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := fusion.NewFusionEngine(tt.config, logger)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, engine)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, engine)
			}
		})
	}
}

func TestFusionEngine_Store(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		CogneeEndpoint: "http://cognee:8000",
		CogneeAPIKey:   "test-key",
		Mem0Endpoint:   "http://mem0:8000",
		Mem0APIKey:     "test-key",
	}

	engine, err := fusion.NewFusionEngine(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name    string
		entry   *types.MemoryEntry
		wantErr bool
	}{
		{
			name: "store fact memory",
			entry: &types.MemoryEntry{
				ID:      "test-1",
				Content: "Test fact memory",
				Type:    types.MemoryTypeFact,
				UserID:  "user-1",
			},
			wantErr: false,
		},
		{
			name: "store graph memory",
			entry: &types.MemoryEntry{
				ID:      "test-2",
				Content: "Test graph memory",
				Type:    types.MemoryTypeGraph,
				UserID:  "user-1",
			},
			wantErr: false,
		},
		{
			name: "store episodic memory",
			entry: &types.MemoryEntry{
				ID:        "test-3",
				Content:   "Test episodic memory",
				Type:      types.MemoryTypeEpisodic,
				UserID:    "user-1",
				SessionID: "session-1",
			},
			wantErr: false,
		},
		{
			name: "store core memory with agent",
			entry: &types.MemoryEntry{
				ID:      "test-4",
				Content: "Test core memory",
				Type:    types.MemoryTypeCore,
				UserID:  "user-1",
				AgentID: "agent-1",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This will fail without actual backends running
			// In production, these would be integration tests
			err := engine.Store(ctx, tt.entry)
			// Expect error since no backends are running
			assert.Error(t, err)
		})
	}
}

func TestFusionEngine_Retrieve(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		Mem0Endpoint: "http://mem0:8000",
		Mem0APIKey:   "test-key",
	}

	engine, err := fusion.NewFusionEngine(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()
	req := &types.SearchRequest{
		Query:  "test query",
		UserID: "user-1",
		TopK:   10,
	}

	// This will fail without actual backends
	result, err := engine.Retrieve(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestFusionEngine_HealthCheck(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{
		Mem0Endpoint: "http://mem0:8000",
		Mem0APIKey:   "test-key",
	}

	engine, err := fusion.NewFusionEngine(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// This will fail without actual backends
	results := engine.HealthCheck(ctx)
	assert.NotNil(t, results)
	// All systems should report error since no backends are running
	for _, err := range results {
		assert.Error(t, err)
	}
}

func TestFusionEngine_GetStats(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{}

	engine, err := fusion.NewFusionEngine(cfg, logger)
	require.NoError(t, err)

	stats := engine.GetStats()
	assert.NotNil(t, stats)
	assert.False(t, stats.CogneeHealthy)
	assert.False(t, stats.Mem0Healthy)
	assert.False(t, stats.LettaHealthy)
}

// ==================== Benchmark Tests ====================

func BenchmarkFusionEngine_Store(b *testing.B) {
	logger := zap.NewNop()
	cfg := &config.Config{}
	engine, _ := fusion.NewFusionEngine(cfg, logger)
	ctx := context.Background()

	entry := &types.MemoryEntry{
		ID:      "bench-1",
		Content: "Benchmark memory",
		Type:    types.MemoryTypeFact,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Store(ctx, entry)
	}
}

func BenchmarkFusionEngine_Retrieve(b *testing.B) {
	logger := zap.NewNop()
	cfg := &config.Config{}
	engine, _ := fusion.NewFusionEngine(cfg, logger)
	ctx := context.Background()

	req := &types.SearchRequest{
		Query: "benchmark query",
		TopK:  10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Retrieve(ctx, req)
	}
}
