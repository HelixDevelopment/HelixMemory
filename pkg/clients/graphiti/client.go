// Package graphiti provides the Graphiti temporal knowledge graph client
// for HelixMemory. Graphiti (by Zep) enables bi-temporal data modeling,
// edge invalidation, and hybrid search for time-aware memory queries.
package graphiti

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

// Client communicates with the Graphiti API for temporal graph operations.
type Client struct {
	endpoint   string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
}

// NewClient creates a Graphiti client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.GraphitiEndpoint,
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
	return types.SourceGraphiti
}

// graphitiEpisode represents a Graphiti episode (a unit of experience).
type graphitiEpisode struct {
	ID        string    `json:"uuid,omitempty"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Source    string     `json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	ValidAt   time.Time `json:"valid_at,omitempty"`
}

// graphitiNode represents a node in the temporal graph.
type graphitiNode struct {
	ID        string                 `json:"uuid"`
	Name      string                 `json:"name"`
	Summary   string                 `json:"summary,omitempty"`
	Labels    []string               `json:"labels,omitempty"`
	CreatedAt time.Time              `json:"created_at,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// graphitiEdge represents a temporal edge in the graph.
type graphitiEdge struct {
	ID         string    `json:"uuid"`
	SourceID   string    `json:"source_node_uuid"`
	TargetID   string    `json:"target_node_uuid"`
	Name       string    `json:"name"`
	Fact       string    `json:"fact,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	ValidAt    time.Time `json:"valid_at,omitempty"`
	InvalidAt  *time.Time `json:"invalid_at,omitempty"`
}

// graphitiSearchRequest is the request for searching the temporal graph.
type graphitiSearchRequest struct {
	Query     string     `json:"query"`
	Limit     int        `json:"num_results,omitempty"`
	CenterAt  *time.Time `json:"center_date,omitempty"`
}

// graphitiSearchResponse is the response from search.
type graphitiSearchResponse struct {
	Nodes []graphitiNode `json:"nodes,omitempty"`
	Edges []graphitiEdge `json:"edges,omitempty"`
}

// Add stores a temporal memory as an episode in Graphiti.
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("graphiti: circuit breaker open")
	}

	episode := &graphitiEpisode{
		Name:    fmt.Sprintf("memory_%s", entry.ID),
		Content: entry.Content,
		Source:  string(entry.Source),
		ValidAt: entry.CreatedAt,
	}

	body, err := json.Marshal(episode)
	if err != nil {
		return fmt.Errorf("graphiti: marshal episode: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/episodes",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("graphiti: create add request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("graphiti: add request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("graphiti: add API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Search queries the temporal graph using hybrid search.
func (c *Client) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("graphiti: circuit breaker open")
	}

	start := time.Now()

	searchReq := &graphitiSearchRequest{
		Query: req.Query,
		Limit: req.TopK,
	}
	if searchReq.Limit <= 0 {
		searchReq.Limit = 10
	}
	if req.TimeRange != nil {
		centerAt := req.TimeRange.Start.Add(req.TimeRange.End.Sub(req.TimeRange.Start) / 2)
		searchReq.CenterAt = &centerAt
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("graphiti: marshal search: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/search",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("graphiti: create search request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("graphiti: search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("graphiti: search API error %d: %s", resp.StatusCode, string(respBody))
	}

	var searchResp graphitiSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("graphiti: decode search response: %w", err)
	}

	c.breaker.RecordSuccess()

	entries := make([]*types.MemoryEntry, 0)

	// Convert edges to memory entries (edges carry temporal facts)
	for _, edge := range searchResp.Edges {
		content := edge.Fact
		if content == "" {
			content = edge.Name
		}
		entry := &types.MemoryEntry{
			ID:         edge.ID,
			Content:    content,
			Type:       types.MemoryTypeTemporal,
			Source:     types.SourceGraphiti,
			Confidence: 0.85,
			Metadata: map[string]interface{}{
				"source_node": edge.SourceID,
				"target_node": edge.TargetID,
				"edge_name":   edge.Name,
				"valid_at":    edge.ValidAt,
			},
			CreatedAt: edge.CreatedAt,
			UpdatedAt: edge.CreatedAt,
		}
		if edge.InvalidAt != nil {
			entry.Metadata["invalid_at"] = *edge.InvalidAt
		}
		entries = append(entries, entry)
	}

	// Convert nodes to memory entries
	for _, node := range searchResp.Nodes {
		content := node.Summary
		if content == "" {
			content = node.Name
		}
		entry := &types.MemoryEntry{
			ID:         node.ID,
			Content:    content,
			Type:       types.MemoryTypeTemporal,
			Source:     types.SourceGraphiti,
			Confidence: 0.80,
			Metadata: map[string]interface{}{
				"node_name":   node.Name,
				"labels":      node.Labels,
				"properties":  node.Properties,
			},
			CreatedAt: node.CreatedAt,
			UpdatedAt: node.CreatedAt,
		}
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}
		entries = append(entries, entry)
	}

	return &types.SearchResult{
		Entries:  entries,
		Total:    len(entries),
		Duration: time.Since(start),
		Sources:  []types.MemorySource{types.SourceGraphiti},
	}, nil
}

// Get retrieves a node by ID from the temporal graph.
func (c *Client) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("graphiti: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/nodes/"+id,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("graphiti: create get request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("graphiti: get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.breaker.RecordSuccess()
		return nil, fmt.Errorf("graphiti: node %s not found", id)
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("graphiti: get API error %d: %s", resp.StatusCode, string(respBody))
	}

	var node graphitiNode
	if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
		return nil, fmt.Errorf("graphiti: decode get response: %w", err)
	}

	c.breaker.RecordSuccess()

	content := node.Summary
	if content == "" {
		content = node.Name
	}

	return &types.MemoryEntry{
		ID:         node.ID,
		Content:    content,
		Type:       types.MemoryTypeTemporal,
		Source:     types.SourceGraphiti,
		Confidence: 0.85,
		Metadata:   node.Properties,
		CreatedAt:  node.CreatedAt,
		UpdatedAt:  node.CreatedAt,
	}, nil
}

// Update modifies a node in the temporal graph.
func (c *Client) Update(ctx context.Context, entry *types.MemoryEntry) error {
	// Graphiti uses episode-based updates; add new episode
	return c.Add(ctx, entry)
}

// Delete removes a node from the temporal graph.
func (c *Client) Delete(ctx context.Context, id string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("graphiti: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/v1/nodes/"+id,
		nil,
	)
	if err != nil {
		return fmt.Errorf("graphiti: create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("graphiti: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("graphiti: delete API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// GetHistory returns temporal memories for a user.
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

// SearchTemporal queries memories with temporal reasoning at a specific time.
func (c *Client) SearchTemporal(ctx context.Context, query string, at time.Time) ([]*types.MemoryEntry, error) {
	req := &types.SearchRequest{
		Query: query,
		TopK:  10,
		TimeRange: &types.TimeRange{
			Start: at.Add(-24 * time.Hour),
			End:   at.Add(24 * time.Hour),
		},
	}
	result, err := c.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// GetTimeline returns a chronological view of memories between two times.
func (c *Client) GetTimeline(ctx context.Context, userID string, start, end time.Time) ([]*types.MemoryEntry, error) {
	req := &types.SearchRequest{
		Query:  "*",
		UserID: userID,
		TopK:   100,
		TimeRange: &types.TimeRange{
			Start: start,
			End:   end,
		},
	}
	result, err := c.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// InvalidateAt marks a temporal edge as invalid at a specific time.
func (c *Client) InvalidateAt(ctx context.Context, id string, at time.Time) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("graphiti: circuit breaker open")
	}

	body, err := json.Marshal(map[string]interface{}{
		"invalid_at": at,
	})
	if err != nil {
		return fmt.Errorf("graphiti: marshal invalidation: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPatch,
		c.endpoint+"/v1/edges/"+id,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("graphiti: create invalidation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("graphiti: invalidation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("graphiti: invalidation error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Health checks if Graphiti is available.
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/health",
		nil,
	)
	if err != nil {
		return fmt.Errorf("graphiti: create health request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("graphiti: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("graphiti: unhealthy (status %d)", resp.StatusCode)
	}

	return nil
}
