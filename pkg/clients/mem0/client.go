// Package mem0 provides the Mem0 backend client for HelixMemory.
// Mem0 is an advanced memory layer for AI applications with user-specific,
// agent-specific, and session-based memory management.
// API Reference: https://docs.mem0.ai/api-reference
package mem0

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// Client communicates with the Mem0 REST API.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
	orgID      string
	projectID  string
}

// NewClient creates a Mem0 client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.Mem0Endpoint,
		apiKey:   cfg.Mem0APIKey,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		breaker: types.NewCircuitBreaker(
			cfg.CircuitBreakerThreshold,
			cfg.CircuitBreakerTimeout,
		),
		orgID:     cfg.Mem0OrgID,
		projectID: cfg.Mem0ProjectID,
	}
}

// Name returns the provider identifier.
func (c *Client) Name() types.MemorySource {
	return types.SourceMem0
}

// ==================== API Request/Response Types ====================

// Memory represents a stored memory in Mem0
type Memory struct {
	ID            string                 `json:"id"`
	Memory        string                 `json:"memory"`
	UserID        string                 `json:"user_id,omitempty"`
	AgentID       string                 `json:"agent_id,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Categories    []string               `json:"categories,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Score         float64                `json:"score,omitempty"`
}

// AddMemoryRequest represents a request to add a memory
type AddMemoryRequest struct {
	Messages   []Message              `json:"messages"`
	UserID     string                 `json:"user_id,omitempty"`
	AgentID    string                 `json:"agent_id,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Categories []string               `json:"categories,omitempty"`
	Immutable  bool                   `json:"immutable,omitempty"`
}

// Message represents a conversation message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// UpdateMemoryRequest represents a request to update a memory
type UpdateMemoryRequest struct {
	MemoryID string `json:"memory_id"`
	Memory   string `json:"memory"`
}

// SearchMemoryRequest represents a request to search memories
type SearchMemoryRequest struct {
	Query     string                 `json:"query"`
	UserID    string                 `json:"user_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Filters   map[string]interface{} `json:"filters,omitempty"`
	TopK      int                    `json:"top_k,omitempty"`
}

// MemoryHistory represents the history of a memory
type MemoryHistory struct {
	ID        string    `json:"id"`
	MemoryID  string    `json:"memory_id"`
	OldValue  string    `json:"old_value"`
	NewValue  string    `json:"new_value"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

// User represents a Mem0 user
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Agent represents a Mem0 agent
type Agent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Config    map[string]interface{} `json:"config,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Webhook represents a Mem0 webhook
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Name      string    `json:"name"`
	Events    []string  `json:"events"`
	Headers   map[string]string `json:"headers,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ==================== Core Memory Operations ====================

// Add stores a new memory in Mem0.
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	req := &AddMemoryRequest{
		Messages: []Message{
			{Role: "user", Content: entry.Content},
		},
		UserID:    entry.UserID,
		AgentID:   entry.AgentID,
		SessionID: entry.SessionID,
		Metadata:  entry.Metadata,
	}

	// Extract categories from metadata
	if cats, ok := entry.Metadata["categories"].([]string); ok {
		req.Categories = cats
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mem0: marshal add request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/memories/",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("mem0: create add request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: add request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: add API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// AddBatch stores multiple memories in a single request.
func (c *Client) AddBatch(ctx context.Context, entries []*types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	// Mem0 batch API uses the same endpoint with multiple messages
	for _, entry := range entries {
		if err := c.Add(ctx, entry); err != nil {
			return fmt.Errorf("mem0: batch add failed for entry %s: %w", entry.ID, err)
		}
	}

	return nil
}

// Get retrieves a memory by ID.
func validateID(id string) error {
	if strings.Contains(id, "\\") || strings.HasPrefix(id, "/") {
		return fmt.Errorf("invalid id: path traversal detected")
	}
	decoded, err := url.PathUnescape(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}
	if strings.Contains(decoded, "..") {
		return fmt.Errorf("invalid id: path traversal detected")
	}
	return nil
}

func (c *Client) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	if err := validateID(id); err != nil {
		return nil, fmt.Errorf("mem0: %w", err)
	}
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
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

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

	var memory Memory
	if err := json.NewDecoder(resp.Body).Decode(&memory); err != nil {
		return nil, fmt.Errorf("mem0: decode get response: %w", err)
	}

	c.breaker.RecordSuccess()
	return c.toMemoryEntry(&memory), nil
}

// Update modifies an existing memory.
func (c *Client) Update(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	req := &UpdateMemoryRequest{
		MemoryID: entry.ID,
		Memory:   entry.Content,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mem0: marshal update request: %w", err)
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
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

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
	if err := validateID(id); err != nil {
		return fmt.Errorf("mem0: %w", err)
	}
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
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// DeleteAll removes all memories for a specific entity.
func (c *Client) DeleteAll(ctx context.Context, userID, agentID string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	url := c.endpoint + "/v1/memories/"
	if userID != "" {
		url += "?user_id=" + userID
	} else if agentID != "" {
		url += "?agent_id=" + agentID
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("mem0: create delete all request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete all request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete all API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Search finds memories matching the query.
func (c *Client) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	start := time.Now()

	searchReq := &SearchMemoryRequest{
		Query:     req.Query,
		UserID:    req.UserID,
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		TopK:      req.TopK,
		Filters:   req.Filter,
	}
	if searchReq.TopK <= 0 {
		searchReq.TopK = 10
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("mem0: marshal search request: %w", err)
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
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

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

	var result struct {
		Results []Memory `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mem0: decode search response: %w", err)
	}
	memories := result.Results

	c.breaker.RecordSuccess()

	entries := make([]*types.MemoryEntry, 0, len(memories))
	for _, m := range memories {
		entries = append(entries, c.toMemoryEntry(&m))
	}

	return &types.SearchResult{
		Entries:  entries,
		Total:    len(entries),
		Duration: time.Since(start),
		Sources:  []types.MemorySource{types.SourceMem0},
	}, nil
}

// GetHistory retrieves all memories for a user.
func (c *Client) GetHistory(ctx context.Context, userID string, limit int) ([]*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	url := c.endpoint + "/v1/memories/?user_id=" + userID
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("mem0: create history request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

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

	var memories []Memory
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

// GetMemoryHistory retrieves the edit history of a specific memory.
func (c *Client) GetMemoryHistory(ctx context.Context, memoryID string) ([]MemoryHistory, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/memories/"+memoryID+"/history/",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create memory history request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: memory history request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: memory history API error %d: %s", resp.StatusCode, string(respBody))
	}

	var history []MemoryHistory
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		return nil, fmt.Errorf("mem0: decode memory history response: %w", err)
	}

	c.breaker.RecordSuccess()
	return history, nil
}

// ==================== User Management ====================

// CreateUser creates a new user.
func (c *Client) CreateUser(ctx context.Context, name string, metadata map[string]interface{}) (*User, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	reqBody := map[string]interface{}{
		"name":     name,
		"metadata": metadata,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("mem0: marshal create user request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/users/",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create user request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: create user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: create user API error %d: %s", resp.StatusCode, string(respBody))
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("mem0: decode create user response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &user, nil
}

// GetUser retrieves a user by ID.
func (c *Client) GetUser(ctx context.Context, userID string) (*User, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/users/"+userID+"/",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create get user request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get user API error %d: %s", resp.StatusCode, string(respBody))
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("mem0: decode get user response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &user, nil
}

// DeleteUser removes a user and all their memories.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/v1/users/"+userID+"/",
		nil,
	)
	if err != nil {
		return fmt.Errorf("mem0: create delete user request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete user API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// ==================== Agent Management ====================

// CreateAgent creates a new agent.
func (c *Client) CreateAgent(ctx context.Context, name string, config map[string]interface{}) (*Agent, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	reqBody := map[string]interface{}{
		"name":   name,
		"config": config,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("mem0: marshal create agent request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/agents/",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create agent request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: create agent request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: create agent API error %d: %s", resp.StatusCode, string(respBody))
	}

	var agent Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, fmt.Errorf("mem0: decode create agent response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &agent, nil
}

// GetAgent retrieves an agent by ID.
func (c *Client) GetAgent(ctx context.Context, agentID string) (*Agent, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/agents/"+agentID+"/",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create get agent request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get agent request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get agent API error %d: %s", resp.StatusCode, string(respBody))
	}

	var agent Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, fmt.Errorf("mem0: decode get agent response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &agent, nil
}

// DeleteAgent removes an agent and all their memories.
func (c *Client) DeleteAgent(ctx context.Context, agentID string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/v1/agents/"+agentID+"/",
		nil,
	)
	if err != nil {
		return fmt.Errorf("mem0: create delete agent request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete agent request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete agent API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// ==================== Webhook Management ====================

// CreateWebhook creates a new webhook.
func (c *Client) CreateWebhook(ctx context.Context, url, name string, events []string) (*Webhook, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	reqBody := map[string]interface{}{
		"url":    url,
		"name":   name,
		"events": events,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("mem0: marshal create webhook request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/webhooks/",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create webhook request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: create webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: create webhook API error %d: %s", resp.StatusCode, string(respBody))
	}

	var webhook Webhook
	if err := json.NewDecoder(resp.Body).Decode(&webhook); err != nil {
		return nil, fmt.Errorf("mem0: decode create webhook response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &webhook, nil
}

// GetWebhook retrieves a webhook by ID.
func (c *Client) GetWebhook(ctx context.Context, webhookID string) (*Webhook, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/webhooks/"+webhookID+"/",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("mem0: create get webhook request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("mem0: get webhook API error %d: %s", resp.StatusCode, string(respBody))
	}

	var webhook Webhook
	if err := json.NewDecoder(resp.Body).Decode(&webhook); err != nil {
		return nil, fmt.Errorf("mem0: decode get webhook response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &webhook, nil
}

// DeleteWebhook removes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("mem0: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/v1/webhooks/"+webhookID+"/",
		nil,
	)
	if err != nil {
		return fmt.Errorf("mem0: create delete webhook request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("mem0: delete webhook API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// ==================== Health Check ====================

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
	httpReq.Header.Set("Authorization", "Token "+c.apiKey)

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

// ==================== Helper Functions ====================

func (c *Client) toMemoryEntry(m *Memory) *types.MemoryEntry {
	entry := &types.MemoryEntry{
		ID:         m.ID,
		Content:    m.Memory,
		UserID:     m.UserID,
		AgentID:    m.AgentID,
		SessionID:  m.SessionID,
		Type:       types.MemoryTypeSemantic,
		Source:     types.SourceMem0,
		Confidence: m.Score,
		Relevance:  m.Score,
		Metadata:   m.Metadata,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}

	// Add categories to metadata
	if len(m.Categories) > 0 {
		entry.Metadata["categories"] = m.Categories
	}

	return entry
}
