package graphiti

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
	cfg.GraphitiEndpoint = serverURL
	return NewClient(cfg)
}

func TestClient_Name(t *testing.T) {
	c := newTestClient("http://localhost")
	assert.Equal(t, types.SourceGraphiti, c.Name())
}

func TestClient_Add(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/episodes", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]interface{}
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)
		assert.NotEmpty(t, req["content"])

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Add(ctx, &types.MemoryEntry{
		ID:        "test-1",
		Content:   "A knows B",
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/search", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"edges":[{"uuid":"e1","name":"knows","fact":"A knows B","source_node_uuid":"n1","target_node_uuid":"n2"}],
			"nodes":[{"uuid":"n1","name":"A","summary":"Entity A"}]
		}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	result, err := c.Search(ctx, &types.SearchRequest{Query: "who knows whom", TopK: 10})
	require.NoError(t, err)
	assert.Len(t, result.Entries, 2)
	for _, entry := range result.Entries {
		assert.Equal(t, types.SourceGraphiti, entry.Source)
	}
}

func TestClient_Search_WithTimeRange(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"edges":[],"nodes":[]}`))
	}))
	defer server.Close()

	now := time.Now()
	start := now.Add(-24 * time.Hour)
	c := newTestClient(server.URL)
	ctx := context.Background()
	_, err := c.Search(ctx, &types.SearchRequest{
		Query: "temporal query",
		TopK:  5,
		TimeRange: &types.TimeRange{
			Start: start,
			End:   now,
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, receivedBody["center_date"])
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/nodes/test-1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uuid":"test-1","name":"A","summary":"Entity A"}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	entry, err := c.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", entry.ID)
	assert.Contains(t, entry.Content, "Entity A")
	assert.Equal(t, types.SourceGraphiti, entry.Source)
}

func TestClient_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	_, err := c.Get(ctx, "test-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClient_Update(t *testing.T) {
	postCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/episodes" {
			postCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Update(ctx, &types.MemoryEntry{ID: "test-1", Content: "updated relation"})
	require.NoError(t, err)
	assert.True(t, postCalled, "Update delegates to Add via POST /v1/episodes")
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/nodes/test-1", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Delete(ctx, "test-1")
	require.NoError(t, err)
}

func TestClient_SearchTemporal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"edges":[],"nodes":[]}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	targetTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	entries, err := c.SearchTemporal(ctx, "what happened", targetTime)
	require.NoError(t, err)
	assert.NotNil(t, entries)
}

func TestClient_GetTimeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"edges":[{"uuid":"e1","name":"event","fact":"something happened"}],"nodes":[]}`))
	}))
	defer server.Close()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	c := newTestClient(server.URL)
	ctx := context.Background()
	entries, err := c.GetTimeline(ctx, "user-1", start, end)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}

func TestClient_InvalidateAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/v1/edges/test-1", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]interface{}
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)
		assert.NotEmpty(t, req["invalid_at"])

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.InvalidateAt(ctx, "test-1", time.Now())
	require.NoError(t, err)
}

func TestClient_Health(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "healthy", statusCode: 200, wantErr: false},
		{name: "unhealthy", statusCode: 500, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/health", r.URL.Path)
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			ctx := context.Background()
			err := c.Health(ctx)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
