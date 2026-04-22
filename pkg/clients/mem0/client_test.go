package mem0

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(serverURL string) *Client {
	cfg := config.DefaultConfig()
	cfg.Mem0Endpoint = serverURL
	return NewClient(cfg)
}

func TestClient_Name(t *testing.T) {
	c := newTestClient("http://localhost")
	assert.Equal(t, types.SourceMem0, c.Name())
}

func TestClient_Add(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/memories/", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"mem-1"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Add(ctx, &types.MemoryEntry{Content: "test memory", UserID: "user-1"})
	require.NoError(t, err)
	assert.Equal(t, "test memory",
		receivedBody["messages"].([]interface{})[0].(map[string]interface{})["content"])
}

func TestClient_Add_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Add(ctx, &types.MemoryEntry{Content: "test", UserID: "user-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/memories/search/", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"id":"1","memory":"test","score":0.9}]}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	result, err := c.Search(ctx, &types.SearchRequest{Query: "test query", TopK: 5, UserID: "user-1"})
	require.NoError(t, err)
	assert.Len(t, result.Entries, 1)
	assert.Equal(t, "1", result.Entries[0].ID)
	assert.Equal(t, types.SourceMem0, result.Entries[0].Source)
}

func TestClient_Search_DefaultTopK(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	_, err := c.Search(ctx, &types.SearchRequest{Query: "test", TopK: 0, UserID: "user-1"})
	require.NoError(t, err)
	assert.Equal(t, float64(10), receivedBody["top_k"])
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/memories/test-1/", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test-1","memory":"content"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	entry, err := c.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", entry.ID)
	assert.Equal(t, "content", entry.Content)
}

func TestClient_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"not found"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	_, err := c.Get(ctx, "test-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClient_Update(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/v1/memories/test-1/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test-1","memory":"updated"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Update(ctx, &types.MemoryEntry{ID: "test-1", Content: "updated"})
	require.NoError(t, err)
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/memories/test-1/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"deleted"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Delete(ctx, "test-1")
	require.NoError(t, err)
}

func TestClient_GetHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/memories/", r.URL.Path)
		assert.Equal(t, "u1", r.URL.Query().Get("user_id"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"m1","memory":"first"},{"id":"m2","memory":"second"}]`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	entries, err := c.GetHistory(ctx, "u1", 10)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "m1", entries[0].ID)
	assert.Equal(t, "m2", entries[1].ID)
}

func TestClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Health(ctx)
	require.NoError(t, err)
}

func TestClient_Health_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Health(ctx)
	require.Error(t, err)
}

func TestClient_CircuitBreaker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Mem0Endpoint = server.URL
	cfg.CircuitBreakerThreshold = 3
	cfg.CircuitBreakerTimeout = 30 * time.Second
	c := NewClient(cfg)
	ctx := context.Background()

	for i := 0; i < cfg.CircuitBreakerThreshold; i++ {
		_ = c.Add(ctx, &types.MemoryEntry{Content: "test", UserID: "user-1"})
	}

	err := c.Add(ctx, &types.MemoryEntry{Content: "test", UserID: "user-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")
}

func TestClient_ToMemoryEntry(t *testing.T) {
	c := newTestClient("http://localhost")

	created, _ := time.Parse(time.RFC3339, "2025-01-15T10:30:00Z")
	updated, _ := time.Parse(time.RFC3339, "2025-01-15T11:00:00Z")

	m := &Memory{
		ID:        "entry-1",
		Memory:    "test content",
		CreatedAt: created,
		UpdatedAt: updated,
		Score:     0.95,
	}

	entry := c.toMemoryEntry(m)
	assert.Equal(t, "entry-1", entry.ID)
	assert.Equal(t, "test content", entry.Content)
	assert.Equal(t, types.MemoryTypeSemantic, entry.Type)
	assert.Equal(t, types.SourceMem0, entry.Source)
	assert.Equal(t, 0.95, entry.Relevance)
	assert.Equal(t, created, entry.CreatedAt)
	assert.Equal(t, updated, entry.UpdatedAt)
}

func TestClient_ToMemoryEntry_EmptyID(t *testing.T) {
	c := newTestClient("http://localhost")
	m := &Memory{Memory: "content without id"}
	entry := c.toMemoryEntry(m)
	assert.NotEmpty(t, entry.ID)
}
