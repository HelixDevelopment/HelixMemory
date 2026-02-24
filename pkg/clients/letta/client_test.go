package letta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(serverURL string) *Client {
	cfg := config.DefaultConfig()
	cfg.LettaEndpoint = serverURL
	return NewClient(cfg)
}

func agentListHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`[{"id":"agent-1","name":"helixmemory"}]`))
}

func TestClient_Name(t *testing.T) {
	c := newTestClient("http://localhost")
	assert.Equal(t, types.SourceLetta, c.Name())
}

func TestClient_EnsureAgent_Existing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/agents/" {
			agentListHandler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	agentID, err := c.EnsureAgent(ctx)
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agentID)
}

func TestClient_EnsureAgent_Create(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agents/":
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			_ = json.Unmarshal(body, &req)
			assert.Equal(t, "helixmemory", req["name"])

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"new-agent","name":"helixmemory"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	agentID, err := c.EnsureAgent(ctx)
	require.NoError(t, err)
	assert.Equal(t, "new-agent", agentID)
}

func TestClient_EnsureAgent_Cached(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/agents/" {
			atomic.AddInt32(&requestCount, 1)
			agentListHandler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()

	agentID1, err := c.EnsureAgent(ctx)
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agentID1)

	agentID2, err := c.EnsureAgent(ctx)
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agentID2)

	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
}

func TestClient_Add(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			agentListHandler(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agents/agent-1/messages":
			body, _ := io.ReadAll(r.Body)
			assert.NotEmpty(t, body)
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

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/agents/" && r.URL.RawQuery == "" {
			agentListHandler(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/agents/") &&
			strings.Contains(r.URL.Path, "/archival") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"1","text":"content"}]`))
			return
		}
		t.Logf("UNMATCHED: %s %s (path=%s query=%s)", r.Method, r.URL.String(), r.URL.Path, r.URL.RawQuery)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	result, err := c.Search(ctx, &types.SearchRequest{Query: "test", TopK: 5})
	require.NoError(t, err)
	assert.Len(t, result.Entries, 1)
	assert.Equal(t, types.SourceLetta, result.Entries[0].Source)
	assert.Equal(t, "content", result.Entries[0].Content)
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			agentListHandler(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/agent-1/archival/test-1":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"test-1","text":"content"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	entry, err := c.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", entry.ID)
	assert.Equal(t, "content", entry.Content)
	assert.Equal(t, types.SourceLetta, entry.Source)
}

func TestClient_Update(t *testing.T) {
	patchCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			agentListHandler(w, r)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/agents/agent-1/archival/test-1":
			patchCalled = true
			body, _ := io.ReadAll(r.Body)
			assert.NotEmpty(t, body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"test-1","text":"updated"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Update(ctx, &types.MemoryEntry{ID: "test-1", Content: "updated content"})
	require.NoError(t, err)
	assert.True(t, patchCalled)
}

func TestClient_Delete(t *testing.T) {
	deleteCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			agentListHandler(w, r)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/agents/agent-1/archival/test-1":
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Delete(ctx, "test-1")
	require.NoError(t, err)
	assert.True(t, deleteCalled)
}

func TestClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/health/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.Health(ctx)
	require.NoError(t, err)
}

func TestClient_GetCoreMemory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			agentListHandler(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/agent-1/memory":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memory":[{"label":"human","value":"test","limit":5000}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	blocks, err := c.GetCoreMemory(ctx, "")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "human", blocks[0].Label)
	assert.Equal(t, "test", blocks[0].Value)
	assert.Equal(t, 5000, blocks[0].Limit)
}

func TestClient_UpdateCoreMemory(t *testing.T) {
	patchCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents/":
			agentListHandler(w, r)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/agents/agent-1/memory":
			patchCalled = true
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			_ = json.Unmarshal(body, &req)
			assert.NotEmpty(t, req)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ctx := context.Background()
	err := c.UpdateCoreMemory(ctx, "", &types.CoreMemoryBlock{
		Label: "human",
		Value: "updated value",
		Limit: 5000,
	})
	require.NoError(t, err)
	assert.True(t, patchCalled)
}
