// Package mem0 provides the Mem0 backend client for HelixMemory.
// Mem0 excels at dynamic fact extraction and preference management,
// delivering 26%+ accuracy improvement over baseline OpenAI memory.
package mem0

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// Client communicates with the Mem0 REST API.
type Client struct {
	endpoint   string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
}

// NewClient creates a Mem0 client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.Mem0Endpoint,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		breaker: types.NewCircuitBreaker(
			cfg.CircuitBreakerThreshold,
			cfg.CircuitBreakerTimeout,
		),
	}
}

// Name returns the provider identifier.
func (c *Client) Name() types.MemorySource {
	return types.SourceMem0
}

// mem0Memory is the Mem0 API memory format.
type mem0Memory struct {
	ID       string                 `json:"id,omitempty"`
	Memory   string                 `json:"memory"`
	UserID   string                 `json:"user_id,omitempty"`
	AgentID  string                 `json:"agent_id,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Hash     string                 `json:"hash,omitempty"`
	Score    float64                `json:"score,omitempty"`
	CreatedAt string               `json:"created_at,omitempty"`
	UpdatedAt string               `json:"updated_at,omitempty"`
}

// mem0AddRequest is the request body for adding memories.
type mem0AddRequest struct {
	Messages []mem0Message          `json:"messages"`
	UserID   string                 `json:"user_id,omitempty"`
	AgentID  string                 `json:"agent_id,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// mem0Message represents a message in Mem0 format.
type mem0Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// mem0SearchRequest is the request body for searching memories.
type mem0SearchRequest struct {
	Query  string `json:"query"`
	UserID string `json:"user_id,omitempty"`
	TopK   int    `json:"top_k,omitempty"`
}

// mem0SearchResponse is the response from memory search.
type mem0SearchResponse struct {
	Results []mem0Memory `json:"results"`
}

// Add stores a memory via Mem0's extraction pipeline.
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	req := &mem0AddRequest{
		Messages: []mem0Message{
			{Role: "user", Content: entry.Content},
		},
		UserID:   entry.UserID,
		AgentID:  entry.AgentID,
		Metadata: entry.Metadata,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mem0: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/memories/",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("mem0: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Search returns memories matching the query.
func (c *Client) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	start := time.Now()

	searchReq := &mem0SearchRequest{
		Query:  req.Query,
		UserID: req.UserID,
		TopK:   req.TopK,
	}
	if searchReq.TopK <= 0 {
		searchReq.TopK = 10
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("mem0: marshal search: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/memories/search/",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: search API error %d: %s", resp.StatusCode, string(respBody))
	}

	var searchResp mem0SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("mem0: decode search response: %w", err)
	}

	c.breaker.RecordSuccess()

	entries := make([]*types.MemoryEntry, 0, len(searchResp.Results))
	for _, m := range searchResp.Results {
		entries = append(entries, c.toMemoryEntry(&m))
	}

	return &types.SearchResult{
		Entries:  entries,
		Total:    len(entries),
		Duration: time.Since(start),
		Sources:  []types.MemorySource{types.SourceMem0},
	}, nil
}

// Get retrieves a memory by ID.
func (c *Client) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/memories/"+id+"/",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create get request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.breaker.RecordSuccess()
		return nil, fmt.Errorf("mem0: memory %s not found", id)
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get API error %d: %s", resp.StatusCode, string(respBody))
	}

	var m mem0Memory
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("mem0: decode get response: %w", err)
	}

	c.breaker.RecordSuccess()
	return c.toMemoryEntry(&m), nil
}

// Update modifies an existing memory.
func (c *Client) Update(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	body, err := json.Marshal(map[string]string{
		"memory": entry.Content,
	})
	if err != nil {
		return fmt.Errorf("mem0: marshal update: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPut,
		c.endpoint+"/v1/memories/"+entry.ID+"/",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("mem0: create update request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: update request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: update API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Delete removes a memory by ID.
func (c *Client) Delete(ctx context.Context, id string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/v1/memories/"+id+"/",
		nil,
	)
	if err != nil {
		return fmt.Errorf("mem0: create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// GetHistory returns memories for a user.
func (c *Client) GetHistory(ctx context.Context, userID string, limit int) ([]*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	url := fmt.Sprintf("%s/v1/memories/?user_id=%s&limit=%d", c.endpoint, userID, limit)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("mem0: create history request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: history request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: history API error %d: %s", resp.StatusCode, string(respBody))
	}

	var memories []mem0Memory
	if err := json.NewDecoder(resp.Body).Decode(&memories); err != nil {
		return nil, fmt.Errorf("mem0: decode history response: %w", err)
	}

	c.breaker.RecordSuccess()

	entries := make([]*types.MemoryEntry, 0, len(memories))
	for _, m := range memories {
		entries = append(entries, c.toMemoryEntry(&m))
	}
	return entries, nil
}

// Health checks if Mem0 is available.
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/health",
		nil,
	)
	if err != nil {
		return fmt.Errorf("mem0: create health request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mem0: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mem0: unhealthy (status %d)", resp.StatusCode)
	}

	return nil
}

// toMemoryEntry converts a Mem0 memory to a unified MemoryEntry.
func (c *Client) toMemoryEntry(m *mem0Memory) *types.MemoryEntry {
	entry := &types.MemoryEntry{
		ID:         m.ID,
		Content:    m.Memory,
		Type:       types.MemoryTypeFact,
		Source:     types.SourceMem0,
		Confidence: 0.85,
		Relevance:  m.Score,
		Metadata:   m.Metadata,
		UserID:     m.UserID,
		AgentID:    m.AgentID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	if m.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
			entry.CreatedAt = t
		}
	}
	if m.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, m.UpdatedAt); err == nil {
			entry.UpdatedAt = t
		}
	}

	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}
	if m.Hash != "" {
		entry.Metadata["mem0_hash"] = m.Hash
	}

	return entry
}
