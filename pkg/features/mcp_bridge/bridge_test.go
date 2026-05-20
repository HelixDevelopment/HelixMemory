package mcp_bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProvider is an in-memory mock of types.MemoryProvider for unit tests.
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
		if req.TopK > 0 && len(entries) >= req.TopK {
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

func TestBridge_ListTools(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	bridge := NewBridge(prov)
	tools := bridge.ListTools()

	expectedNames := []string{
		"memory_search",
		"memory_add",
		"memory_health",
		"memory_get",
		"memory_delete",
	}

	require.Len(t, tools, 5)
	for i, tool := range tools {
		assert.Equal(t, expectedNames[i], tool.Name,
			"tool at index %d has wrong name", i)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.InputSchema)
	}
}

func TestBridge_HandleToolCall_Search(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["e1"] = &types.MemoryEntry{
		ID:      "e1",
		Content: "test fact",
		Type:    types.MemoryTypeFact,
	}

	bridge := NewBridge(prov)
	call := &ToolCall{
		Name:  "memory_search",
		Input: json.RawMessage(`{"query":"test","top_k":5}`),
	}

	result := bridge.HandleToolCall(context.Background(), call)

	assert.False(t, result.IsError, "expected no error, got: %s", result.Content)
	assert.NotEmpty(t, result.Content)

	// Content should be valid JSON containing search results
	var parsed types.SearchResult
	err := json.Unmarshal([]byte(result.Content), &parsed)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, parsed.Total, 1)
}

func TestBridge_HandleToolCall_Add(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	bridge := NewBridge(prov)

	call := &ToolCall{
		Name:  "memory_add",
		Input: json.RawMessage(`{"content":"test fact","type":"fact"}`),
	}

	result := bridge.HandleToolCall(context.Background(), call)

	assert.False(t, result.IsError)
	assert.Equal(t, "memory added successfully", result.Content)
	assert.Len(t, prov.entries, 1, "entry should be stored in provider")
}

func TestBridge_HandleToolCall_Health(t *testing.T) {
	tests := []struct {
		name       string
		healthy    bool
		wantError  bool
		wantSubstr string
	}{
		{
			name:       "healthy provider",
			healthy:    true,
			wantError:  false,
			wantSubstr: "healthy",
		},
		{
			name:       "unhealthy provider",
			healthy:    false,
			wantError:  true,
			wantSubstr: "unhealthy",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov := newTestProvider(types.SourceFusion)
			prov.healthy = tc.healthy
			bridge := NewBridge(prov)

			call := &ToolCall{
				Name:  "memory_health",
				Input: json.RawMessage(`{}`),
			}

			result := bridge.HandleToolCall(context.Background(), call)

			assert.Equal(t, tc.wantError, result.IsError)
			assert.Contains(t, strings.ToLower(result.Content), tc.wantSubstr)
		})
	}
}

func TestBridge_HandleToolCall_Get(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["test-1"] = &types.MemoryEntry{
		ID:      "test-1",
		Content: "stored fact",
		Type:    types.MemoryTypeFact,
		Source:  types.SourceMem0,
	}

	bridge := NewBridge(prov)
	call := &ToolCall{
		Name:  "memory_get",
		Input: json.RawMessage(`{"id":"test-1"}`),
	}

	result := bridge.HandleToolCall(context.Background(), call)

	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "stored fact")
	assert.Contains(t, result.Content, "test-1")
}

func TestBridge_HandleToolCall_Delete(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	prov.entries["test-1"] = &types.MemoryEntry{
		ID:      "test-1",
		Content: "to be deleted",
	}

	bridge := NewBridge(prov)
	call := &ToolCall{
		Name:  "memory_delete",
		Input: json.RawMessage(`{"id":"test-1"}`),
	}

	result := bridge.HandleToolCall(context.Background(), call)

	assert.False(t, result.IsError)
	assert.Contains(t, result.Content, "deleted successfully")
	assert.Empty(t, prov.entries, "entry should be removed from provider")
}

func TestBridge_HandleToolCall_Unknown(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	bridge := NewBridge(prov)

	call := &ToolCall{
		Name:  "memory_nonexistent",
		Input: json.RawMessage(`{}`),
	}

	result := bridge.HandleToolCall(context.Background(), call)

	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "unknown tool")
}

func TestBridge_HandleToolCall_InvalidJSON(t *testing.T) {
	prov := newTestProvider(types.SourceFusion)
	bridge := NewBridge(prov)

	call := &ToolCall{
		Name:  "memory_search",
		Input: json.RawMessage(`{not valid json`),
	}

	result := bridge.HandleToolCall(context.Background(), call)

	assert.True(t, result.IsError)
	assert.Contains(t, result.Content, "invalid input")
}

// TestBridge_ResultMsg_I18nSeam is the CONST-046 round-437 proof: every
// ToolResult.Content string is resolved through the i18n seam, not a hardcoded
// literal. resultMsg MUST return the bundle-backed English text (non-empty,
// non-verbatim key). Paired mutation: deleting any key from en.yaml flips its
// lookup to the verbatim key string and FAILs this test.
func TestBridge_ResultMsg_I18nSeam(t *testing.T) {
	cases := []struct {
		key      string
		args     []interface{}
		wantSub  string
		wantArgs []string
	}{
		{"mcp_result_unknown_tool", []interface{}{"memory_x"}, "unknown tool", []string{"memory_x"}},
		{"mcp_result_invalid_input", []interface{}{"bad"}, "invalid input", []string{"bad"}},
		{"mcp_result_search_error", []interface{}{"boom"}, "search error", []string{"boom"}},
		{"mcp_result_add_error", []interface{}{"boom"}, "add error", []string{"boom"}},
		{"mcp_result_get_error", []interface{}{"boom"}, "get error", []string{"boom"}},
		{"mcp_result_delete_error", []interface{}{"boom"}, "delete error", []string{"boom"}},
		{"mcp_result_unhealthy", []interface{}{"down"}, "unhealthy", []string{"down"}},
		{"mcp_result_add_success", nil, "memory added successfully", nil},
		{"mcp_result_health_ok", nil, "all memory backends healthy", nil},
		{"mcp_result_delete_success", nil, "memory deleted successfully", nil},
	}
	for _, tc := range cases {
		got := resultMsg(tc.key, tc.args...)
		require.NotEmpty(t, got, "key %q resolved empty", tc.key)
		assert.NotEqual(t, tc.key, got, "key %q resolved verbatim (not in bundle)", tc.key)
		assert.Contains(t, got, tc.wantSub, "key %q missing expected substring", tc.key)
		for _, a := range tc.wantArgs {
			assert.Contains(t, got, a, "key %q did not substitute arg %q", tc.key, a)
		}
	}
}
