// Package letta provides the Letta backend client for HelixMemory.
// Letta is the "brain" — a stateful agent runtime with editable in-context
// memory blocks, tool-calling, and sleep-time compute capabilities.
package letta

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

// Client communicates with the Letta server API.
type Client struct {
	endpoint   string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
	agentID    string
}

// NewClient creates a Letta client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.LettaEndpoint,
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
	return types.SourceLetta
}

// lettaAgent represents a Letta agent.
type lettaAgent struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	MemoryBlocks []lettaBlock  `json:"memory_blocks,omitempty"`
	CreatedAt    string        `json:"created_at,omitempty"`
}

// lettaBlock represents a Letta memory block.
type lettaBlock struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Limit int    `json:"limit,omitempty"`
}

// lettaMessage represents a Letta message.
type lettaMessage struct {
	Role    string `json:"role"`
	Content string `json:"text"`
}

// lettaSendRequest is the request to send a message to an agent.
type lettaSendRequest struct {
	Messages []lettaMessage `json:"messages"`
}

// lettaSendResponse is the response from sending a message.
type lettaSendResponse struct {
	Messages []lettaMessage `json:"messages"`
}

// lettaCreateAgentRequest is the request to create a new agent.
type lettaCreateAgentRequest struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	MemoryBlocks []lettaBlock `json:"memory_blocks,omitempty"`
	LLMConfig    interface{}  `json:"llm_config,omitempty"`
}

// EnsureAgent creates or retrieves the HelixMemory agent.
func (c *Client) EnsureAgent(ctx context.Context) (string, error) {
	if c.agentID != "" {
		return c.agentID, nil
	}

	// List agents to find existing HelixMemory agent
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/agents/",
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("letta: create list request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("letta: list agents failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 400 {
		var agents []lettaAgent
		if err := json.NewDecoder(resp.Body).Decode(&agents); err == nil {
			for _, a := range agents {
				if a.Name == "helixmemory" {
					c.agentID = a.ID
					return c.agentID, nil
				}
			}
		}
	}

	// Create new agent
	createReq := &lettaCreateAgentRequest{
		Name:        "helixmemory",
		Description: "HelixMemory unified cognitive memory agent",
		MemoryBlocks: []lettaBlock{
			{Label: "human", Value: "The user interacting with HelixAgent.", Limit: 5000},
			{Label: "persona", Value: "I am the HelixMemory system, managing unified cognitive memory across Mem0, Cognee, and Letta backends.", Limit: 5000},
			{Label: "project_context", Value: "", Limit: 10000},
			{Label: "working_memory", Value: "", Limit: 10000},
		},
	}

	body, err := json.Marshal(createReq)
	if err != nil {
		return "", fmt.Errorf("letta: marshal create request: %w", err)
	}

	createHTTP, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/agents/",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("letta: create agent request: %w", err)
	}
	createHTTP.Header.Set("Content-Type", "application/json")

	createResp, err := c.httpClient.Do(createHTTP)
	if err != nil {
		return "", fmt.Errorf("letta: create agent failed: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(createResp.Body)
		return "", fmt.Errorf("letta: create agent error %d: %s", createResp.StatusCode, string(respBody))
	}

	var agent lettaAgent
	if err := json.NewDecoder(createResp.Body).Decode(&agent); err != nil {
		return "", fmt.Errorf("letta: decode agent response: %w", err)
	}

	c.agentID = agent.ID
	return c.agentID, nil
}

// Add stores a memory by sending it to the Letta agent.
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	agentID, err := c.EnsureAgent(ctx)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: ensure agent: %w", err)
	}

	sendReq := &lettaSendRequest{
		Messages: []lettaMessage{
			{Role: "user", Content: fmt.Sprintf("[MEMORY_STORE] %s", entry.Content)},
		},
	}

	body, err := json.Marshal(sendReq)
	if err != nil {
		return fmt.Errorf("letta: marshal send request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		fmt.Sprintf("%s/v1/agents/%s/messages", c.endpoint, agentID),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("letta: create send request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: send request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: send API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Search queries the Letta agent for relevant memories.
func (c *Client) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	start := time.Now()

	agentID, err := c.EnsureAgent(ctx)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: ensure agent: %w", err)
	}

	// Use archival memory search
	searchURL := fmt.Sprintf(
		"%s/v1/agents/%s/archival?query=%s&limit=%d",
		c.endpoint, agentID, req.Query, req.TopK,
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("letta: create search request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: search API error %d: %s", resp.StatusCode, string(respBody))
	}

	var results []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("letta: decode search response: %w", err)
	}

	c.breaker.RecordSuccess()

	entries := make([]*types.MemoryEntry, 0, len(results))
	for _, r := range results {
		entry := &types.MemoryEntry{
			ID:         fmt.Sprintf("%v", r["id"]),
			Type:       types.MemoryTypeCore,
			Source:     types.SourceLetta,
			Confidence: 0.90,
			Metadata:   r,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if content, ok := r["text"].(string); ok {
			entry.Content = content
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
		Sources:  []types.MemorySource{types.SourceLetta},
	}, nil
}

// Get retrieves a memory by ID from archival storage.
func (c *Client) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	agentID, err := c.EnsureAgent(ctx)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: ensure agent: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/agents/%s/archival/%s", c.endpoint, agentID, id),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create get request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.breaker.RecordSuccess()
		return nil, fmt.Errorf("letta: memory %s not found", id)
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("letta: decode get response: %w", err)
	}

	c.breaker.RecordSuccess()

	entry := &types.MemoryEntry{
		ID:         id,
		Type:       types.MemoryTypeCore,
		Source:     types.SourceLetta,
		Confidence: 0.90,
		Metadata:   result,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if content, ok := result["text"].(string); ok {
		entry.Content = content
	}
	return entry, nil
}

// Update modifies a memory in archival storage.
func (c *Client) Update(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	agentID, err := c.EnsureAgent(ctx)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: ensure agent: %w", err)
	}

	body, err := json.Marshal(map[string]string{"text": entry.Content})
	if err != nil {
		return fmt.Errorf("letta: marshal update: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPatch,
		fmt.Sprintf("%s/v1/agents/%s/archival/%s", c.endpoint, agentID, entry.ID),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("letta: create update request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: update request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: update API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Delete removes a memory from archival storage.
func (c *Client) Delete(ctx context.Context, id string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	agentID, err := c.EnsureAgent(ctx)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: ensure agent: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		fmt.Sprintf("%s/v1/agents/%s/archival/%s", c.endpoint, agentID, id),
		nil,
	)
	if err != nil {
		return fmt.Errorf("letta: create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: delete API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// GetHistory returns recent messages from the Letta agent.
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

// GetCoreMemory retrieves core memory blocks for an agent.
func (c *Client) GetCoreMemory(ctx context.Context, agentID string) ([]*types.CoreMemoryBlock, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	if agentID == "" {
		var err error
		agentID, err = c.EnsureAgent(ctx)
		if err != nil {
			return nil, err
		}
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/agents/%s/memory", c.endpoint, agentID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create core memory request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: core memory request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: core memory API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Memory []lettaBlock `json:"memory"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("letta: decode core memory: %w", err)
	}

	c.breaker.RecordSuccess()

	blocks := make([]*types.CoreMemoryBlock, len(result.Memory))
	for i, b := range result.Memory {
		blocks[i] = &types.CoreMemoryBlock{
			Label:   b.Label,
			Value:   b.Value,
			Limit:   b.Limit,
			AgentID: agentID,
		}
	}
	return blocks, nil
}

// UpdateCoreMemory updates a core memory block.
func (c *Client) UpdateCoreMemory(ctx context.Context, agentID string, block *types.CoreMemoryBlock) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	if agentID == "" {
		var err error
		agentID, err = c.EnsureAgent(ctx)
		if err != nil {
			return err
		}
	}

	body, err := json.Marshal(map[string]interface{}{
		"label": block.Label,
		"value": block.Value,
		"limit": block.Limit,
	})
	if err != nil {
		return fmt.Errorf("letta: marshal core memory update: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPatch,
		fmt.Sprintf("%s/v1/agents/%s/memory", c.endpoint, agentID),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("letta: create core memory update request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: core memory update failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: core memory update error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// Health checks if Letta is available.
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/health/",
		nil,
	)
	if err != nil {
		return fmt.Errorf("letta: create health request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("letta: health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("letta: unhealthy (status %d)", resp.StatusCode)
	}

	return nil
}
