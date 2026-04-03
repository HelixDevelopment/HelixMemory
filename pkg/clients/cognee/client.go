// Package cognee provides the Cognee backend client for HelixMemory.
// Updated for Cognee v1 API (github.com/topoteretes/cognee)
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

// Client communicates with the Cognee REST API v1.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
}

// NewClient creates a Cognee client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.CogneeEndpoint,
		apiKey:   cfg.CogneeAPIKey,
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

// Request/Response types for Cognee v1 API

type cogneeAddRequest struct {
	Data       string            `json:"data"`
	DataType   string            `json:"data_type"`
	DatasetID  string            `json:"dataset_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type cogneeAddResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type cogneeSearchRequest struct {
	Query      string `json:"query"`
	SearchType string `json:"search_type"`
	TopK       int    `json:"top_k,omitempty"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
}

type cogneeSearchResult struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Score       float64                `json:"score"`
	NodeType    string                 `json:"node_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Connections []cogneeConnection     `json:"connections,omitempty"`
	DatasetID   string                 `json:"dataset_id,omitempty"`
}

type cogneeConnection struct {
	TargetID     string  `json:"target_id"`
	RelationType string  `json:"relation_type"`
	Weight       float64 `json:"weight"`
}

type cogneeCognifyRequest struct {
	DatasetID   string   `json:"dataset_id,omitempty"`
	DataSources []string `json:"data_sources,omitempty"`
}

type cogneeCognifyResponse struct {
	Status  string `json:"status"`
	TaskID  string `json:"task_id,omitempty"`
	Message string `json:"message,omitempty"`
}

type cogneeHealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type cogneeDataset struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Add stores content via Cognee's ECL pipeline (v1 API).
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("cognee: circuit breaker open")
	}

	addReq := &cogneeAddRequest{
		Data:     entry.Content,
		DataType: "text",
		Metadata: entry.Metadata,
	}

	// Use dataset from metadata if available
	if datasetID, ok := entry.Metadata["cognee_dataset_id"].(string); ok {
		addReq.DatasetID = datasetID
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
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

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

	var addResp cogneeAddResponse
	if err := json.NewDecoder(resp.Body).Decode(&addResp); err != nil {
		// Non-fatal: data was added but response parsing failed
		c.breaker.RecordSuccess()
		return nil
	}

	// Trigger cognify for the data
	cognifyReq := &cogneeCognifyRequest{}
	if addReq.DatasetID != "" {
		cognifyReq.DatasetID = addReq.DatasetID
	}

	cognifyBody, _ := json.Marshal(cognifyReq)
	cognifyHTTPReq, _ := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/api/v1/cognify",
		bytes.NewReader(cognifyBody),
	)
	cognifyHTTPReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		cognifyHTTPReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	cognifyResp, err := c.httpClient.Do(cognifyHTTPReq)
	if err != nil {
		// Cognify failure is non-fatal; data was added
		c.breaker.RecordSuccess()
		return nil
	}
	defer cognifyResp.Body.Close()

	c.breaker.RecordSuccess()
	return nil
}

// Search returns memories matching the query using Cognee's graph search (v1 API).
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

	// Add dataset filter if specified
	if req.Filter != nil {
		if dsID, ok := req.Filter["dataset_id"].(string); ok {
			searchReq.DatasetIDs = []string{dsID}
		}
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
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

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

// Get retrieves a memory by ID (v1 API).
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
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
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

// Delete removes a memory by ID (v1 API).
func (c *Client) Delete(ctx context.Context, id string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("cognee: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/api/v1/delete/data/"+id,
		nil,
	)
	if err != nil {
		return fmt.Errorf("cognee: create delete request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
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

// Health checks if Cognee is available (v1 API).
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/api/v1/health",
		nil,
	)
	if err != nil {
		return fmt.Errorf("cognee: create health request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cognee: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("cognee: unhealthy (status %d)", resp.StatusCode)
	}

	var healthResp cogneeHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		return fmt.Errorf("cognee: decode health response: %w", err)
	}

	if healthResp.Status != "healthy" && healthResp.Status != "ok" {
		return fmt.Errorf("cognee: unhealthy (status: %s)", healthResp.Status)
	}

	return nil
}

// CreateDataset creates a new dataset in Cognee (v1 API feature).
func (c *Client) CreateDataset(ctx context.Context, name, description string) (*cogneeDataset, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("cognee: circuit breaker open")
	}

	reqBody := map[string]string{
		"name":        name,
		"description": description,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("cognee: marshal dataset request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/api/v1/datasets",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("cognee: create dataset request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: dataset request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: dataset API error %d: %s", resp.StatusCode, string(respBody))
	}

	var dataset cogneeDataset
	if err := json.NewDecoder(resp.Body).Decode(&dataset); err != nil {
		return nil, fmt.Errorf("cognee: decode dataset response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &dataset, nil
}

// ListDatasets returns all datasets (v1 API feature).
func (c *Client) ListDatasets(ctx context.Context) ([]cogneeDataset, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("cognee: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/api/v1/datasets",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("cognee: create list datasets request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: list datasets request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("cognee: list datasets API error %d: %s", resp.StatusCode, string(respBody))
	}

	var datasets []cogneeDataset
	if err := json.NewDecoder(resp.Body).Decode(&datasets); err != nil {
		return nil, fmt.Errorf("cognee: decode datasets response: %w", err)
	}

	c.breaker.RecordSuccess()
	return datasets, nil
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
	if r.DatasetID != "" {
		entry.Metadata["cognee_dataset_id"] = r.DatasetID
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
