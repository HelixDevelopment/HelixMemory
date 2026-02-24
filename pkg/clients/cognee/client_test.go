package cognee

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(serverURL string) *Client {
	cfg := config.DefaultConfig()
	cfg.CogneeEndpoint = serverURL
	return NewClient(cfg)
}

func TestClient_Name(t *testing.T) {
	c := newTestClient("http://localhost")
	assert.Equal(t, types.SourceCognee, c.Name())
}

func TestClient_Add(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/add":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.NotEmpty(t, body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cognify":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Add(ctx, &types.MemoryEntry{Content: "test memory", UserID: "user-1"})
	require.NoError(t, err)
}

func TestClient_Add_CognifyFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cognify":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"cognify failed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Add(ctx, &types.MemoryEntry{Content: "test", UserID: "user-1"})
	// Cognify failure is non-fatal
	require.NoError(t, err)
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/search", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"1","content":"test","score":0.8,"node_type":"Entity"}]`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	result, err := c.Search(ctx, &types.SearchRequest{Query: "test query", TopK: 5, UserID: "user-1"})
	require.NoError(t, err)
	assert.Len(t, result.Entries, 1)
	assert.Equal(t, types.MemoryTypeGraph, result.Entries[0].Type)
	assert.Equal(t, types.SourceCognee, result.Entries[0].Source)
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/data/test-1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test-1","content":"cognee content","node_type":"Entity"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	entry, err := c.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", entry.ID)
	assert.Equal(t, "cognee content", entry.Content)
	assert.Equal(t, types.SourceCognee, entry.Source)
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
	requestMethods := make([]string, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMethods = append(requestMethods, r.Method)
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/data/test-1":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/add":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/cognify":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Update(ctx, &types.MemoryEntry{ID: "test-1", Content: "updated content"})
	require.NoError(t, err)
	assert.Contains(t, requestMethods, http.MethodDelete)
	assert.Contains(t, requestMethods, http.MethodPost)
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/data/test-1", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Delete(ctx, "test-1")
	require.NoError(t, err)
}

func TestClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Health(ctx)
	require.NoError(t, err)
}

func TestClient_ToMemoryEntry(t *testing.T) {
	c := newTestClient("http://localhost")

	r := &cogneeSearchResult{
		ID:       "entry-1",
		Content:  "test content",
		Score:    0.85,
		NodeType: "Entity",
		Connections: []cogneeConnection{
			{TargetID: "node-3", RelationType: "related_to", Weight: 0.9},
		},
	}

	entry := c.toMemoryEntry(r)
	assert.Equal(t, "entry-1", entry.ID)
	assert.Equal(t, "test content", entry.Content)
	assert.Equal(t, types.MemoryTypeGraph, entry.Type)
	assert.Equal(t, types.SourceCognee, entry.Source)
	assert.Equal(t, "Entity", entry.Metadata["cognee_node_type"])
	assert.NotNil(t, entry.Metadata["cognee_connections"])
}

func TestClient_ToMemoryEntry_EmptyID(t *testing.T) {
	c := newTestClient("http://localhost")
	r := &cogneeSearchResult{Content: "content without id"}
	entry := c.toMemoryEntry(r)
	assert.NotEmpty(t, entry.ID)
}
