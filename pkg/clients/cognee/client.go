// Package cognee provides the Cognee backend client for HelixMemory.
// Cognee excels at semantic knowledge graphs via ECL (Extract-Cognify-Load)
// pipelines with 38+ data source connectors and graph-based retrieval.
package cognee

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

// Client communicates with the Cognee REST API.
type Client struct {
	endpoint   string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
}

// NewClient creates a Cognee client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.CogneeEndpoint,
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
	return types.SourceCognee
}

// cogneeAddRequest is the request for adding data to Cognee.
type cogneeAddRequest struct {
	Data     string `json:"data"`
	DataType string `json:"data_type"`
}

// cogneeSearchRequest is the request for searching Cognee.
type cogneeSearchRequest struct {
	Query      string `json:"query"`
	SearchType string `json:"search_type"`
	TopK       int    `json:"top_k,omitempty"`
}

// cogneeSearchResult represents a single Cognee search result.
type cogneeSearchResult struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Score       float64                `json:"score"`
	NodeType    string                 `json:"node_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Connections []cogneeConnection     `json:"connections,omitempty"`
}

// cogneeConnection represents a graph connection.
type cogneeConnection struct {
	TargetID     string  `json:"target_id"`
	RelationType string  `json:"relation_type"`
	Weight       float64 `json:"weight"`
}

// cogneeCognifyResponse is the response from the cognify endpoint.
type cogneeCognifyResponse struct {
	Status string `json:"status"`
	TaskID string `json:"task_id,omitempty"`
}

// Add stores content via Cognee's ECL pipeline.
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("cognee: circuit breaker open")
	}

	// Step 1: Add data
	addReq := &cogneeAddRequest{
		Data:     entry.Content,
		DataType: "text",
	}

	body, err := json.Marshal(addReq)
	if err != nil {
		return fmt.Errorf("cognee: marshal add request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/api/v1/add",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("cognee: create add request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("cognee: add request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("cognee: add API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Step 2: Trigger cognify (ECL pipeline)
	cognifyReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/api/v1/cognify",
		nil,
	)
	if err != nil {
		return fmt.Errorf("cognee: create cognify request: %w", err)
	}

	cognifyResp, err := c.httpClient.Do(cognifyReq)
	if err != nil {
		// Cognify failure is non-fatal; data was added
		c.breaker.RecordSuccess()
		return nil
	}
	defer cognifyResp.Body.Close()

	c.breaker.RecordSuccess()
	return nil
}

// Search returns memories matching the query using Cognee's graph search.
func (c *Client) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("cognee: circuit breaker open")
	}

	start := time.Now()

	searchType := "GRAPH_COMPLETION"
	if req.Filter != nil {
		if st, ok := req.Filter["search_type"].(string); ok {
			searchType = st
		}
	}

	searchReq := &cogneeSearchRequest{
		Query:      req.Query,
		SearchType: searchType,
		TopK:       req.TopK,
	}
	if searchReq.TopK <= 0 {
		searchReq.TopK = 10
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("cognee: marshal search: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/api/v1/search",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("cognee: create search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: search API error %d: %s", resp.StatusCode, string(respBody))
	}

	var results []cogneeSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("cognee: decode search response: %w", err)
	}

	c.breaker.RecordSuccess()

	entries := make([]*types.MemoryEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, c.toMemoryEntry(&r))
	}

	return &types.SearchResult{
		Entries:  entries,
		Total:    len(entries),
		Duration: time.Since(start),
		Sources:  []types.MemorySource{types.SourceCognee},
	}, nil
}

// Get retrieves a memory by ID (via search).
func (c *Client) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("cognee: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/api/v1/data/"+id,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("cognee: create get request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.breaker.RecordSuccess()
		return nil, fmt.Errorf("cognee: memory %s not found", id)
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: get API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result cogneeSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("cognee: decode get response: %w", err)
	}

	c.breaker.RecordSuccess()
	return c.toMemoryEntry(&result), nil
}

// Update modifies a memory (delete + re-add via ECL).
func (c *Client) Update(ctx context.Context, entry *types.MemoryEntry) error {
	if err := c.Delete(ctx, entry.ID); err != nil {
		return fmt.Errorf("cognee: update (delete phase): %w", err)
	}
	return c.Add(ctx, entry)
}

// Delete removes a memory by ID.
func (c *Client) Delete(ctx context.Context, id string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("cognee: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/api/v1/data/"+id,
		nil,
	)
	if err != nil {
		return fmt.Errorf("cognee: create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("cognee: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("cognee: delete API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// GetHistory returns memories for a user (Cognee is project-scoped).
func (c *Client) GetHistory(ctx context.Context, userID string, limit int) ([]*types.MemoryEntry, error) {
	req := &types.SearchRequest{
		Query:  "*",
		UserID: userID,
		TopK:   limit,
	}
	result, err := c.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// Health checks if Cognee is available.
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/api/v1/health",
		nil,
	)
	if err != nil {
		return fmt.Errorf("cognee: create health request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cognee: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("cognee: unhealthy (status %d)", resp.StatusCode)
	}

	return nil
}

// toMemoryEntry converts a Cognee search result to a unified MemoryEntry.
func (c *Client) toMemoryEntry(r *cogneeSearchResult) *types.MemoryEntry {
	entry := &types.MemoryEntry{
		ID:         r.ID,
		Content:    r.Content,
		Type:       types.MemoryTypeGraph,
		Source:     types.SourceCognee,
		Confidence: 0.80,
		Relevance:  r.Score,
		Metadata:   r.Metadata,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}
	if r.NodeType != "" {
		entry.Metadata["cognee_node_type"] = r.NodeType
	}
	if len(r.Connections) > 0 {
		connData := make([]map[string]interface{}, len(r.Connections))
		for i, conn := range r.Connections {
			connData[i] = map[string]interface{}{
				"target_id":     conn.TargetID,
				"relation_type": conn.RelationType,
				"weight":        conn.Weight,
			}
		}
		entry.Metadata["cognee_connections"] = connData
	}

	return entry
}
