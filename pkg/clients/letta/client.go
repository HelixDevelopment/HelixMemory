// Package letta provides the Letta backend client for HelixMemory.
// Letta is an agent memory system with support for persistent agent state,
// tool execution, and multi-agent conversations.
// API: https://docs.letta.com/api-reference
// SDK: https://github.com/letta-ai/letta-code-sdk/
package letta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
)

// Client communicates with the Letta REST API.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	breaker    *types.CircuitBreaker
}

// NewClient creates a Letta client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		endpoint: cfg.LettaEndpoint,
		apiKey:   cfg.LettaAPIKey,
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

// ==================== API Request/Response Types ====================

// Agent represents a Letta agent
type Agent struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	SystemPrompt   string                 `json:"system_prompt,omitempty"`
	Model          string                 `json:"model"`
	MemoryBlocks   []MemoryBlock          `json:"memory_blocks,omitempty"`
	Tools          []Tool                 `json:"tools,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// MemoryBlock represents a memory block in Letta
type MemoryBlock struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Label     string    `json:"label"`
	Value     string    `json:"value"`
	Limit     int       `json:"limit,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tool represents a tool available to the agent
type Tool struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	SourceType  string                 `json:"source_type"`
	SourceCode  string                 `json:"source_code,omitempty"`
	JSONSchema  map[string]interface{} `json:"json_schema,omitempty"`
}

// Message represents a message in a conversation
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	AgentID   string    `json:"agent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAgentRequest represents a request to create an agent
type CreateAgentRequest struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	SystemPrompt string        `json:"system_prompt,omitempty"`
	Model        string        `json:"model"`
	MemoryBlocks []MemoryBlock `json:"memory_blocks,omitempty"`
	Tools        []Tool        `json:"tools,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// SendMessageRequest represents a request to send a message
type SendMessageRequest struct {
	Input     string `json:"input"`
	AgentID   string `json:"agent_id"`
	Stream    bool   `json:"stream,omitempty"`
}

// SendMessageResponse represents a response from sending a message
type SendMessageResponse struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Role    string `json:"role"`
}

// Memory represents a stored memory in Letta
type Memory struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	AgentID   string    `json:"agent_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Source represents a data source for RAG
type Source struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ==================== Agent Management ====================

// CreateAgent creates a new Letta agent.
func (c *Client) CreateAgent(ctx context.Context, req *CreateAgentRequest) (*Agent, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("letta: marshal create agent request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/agents",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create agent request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: create agent request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: create agent API error %d: %s", resp.StatusCode, string(respBody))
	}

	var agent Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, fmt.Errorf("letta: decode create agent response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &agent, nil
}

// GetAgent retrieves an agent by ID.
func (c *Client) GetAgent(ctx context.Context, agentID string) (*Agent, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/agents/"+agentID,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create get agent request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get agent request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get agent API error %d: %s", resp.StatusCode, string(respBody))
	}

	var agent Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, fmt.Errorf("letta: decode get agent response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &agent, nil
}

// ListAgents returns all agents.
func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/agents",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create list agents request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: list agents request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: list agents API error %d: %s", resp.StatusCode, string(respBody))
	}

	var agents []Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("letta: decode list agents response: %w", err)
	}

	c.breaker.RecordSuccess()
	return agents, nil
}

// DeleteAgent removes an agent.
func (c *Client) DeleteAgent(ctx context.Context, agentID string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		c.endpoint+"/v1/agents/"+agentID,
		nil,
	)
	if err != nil {
		return fmt.Errorf("letta: create delete agent request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: delete agent request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: delete agent API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// ==================== Message Operations ====================

// SendMessage sends a message to an agent.
func (c *Client) SendMessage(ctx context.Context, agentID, input string) (*SendMessageResponse, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	req := &SendMessageRequest{
		Input:   input,
		AgentID: agentID,
		Stream:  false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("letta: marshal send message request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/agents/"+agentID+"/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create send message request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: send message request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: send message API error %d: %s", resp.StatusCode, string(respBody))
	}

	var messageResp SendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&messageResp); err != nil {
		return nil, fmt.Errorf("letta: decode send message response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &messageResp, nil
}

// GetMessages retrieves all messages for an agent.
func (c *Client) GetMessages(ctx context.Context, agentID string) ([]Message, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/agents/"+agentID+"/messages",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create get messages request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get messages request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get messages API error %d: %s", resp.StatusCode, string(respBody))
	}

	var messages []Message
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("letta: decode get messages response: %w", err)
	}

	c.breaker.RecordSuccess()
	return messages, nil
}

// ==================== Memory Block Operations ====================

// GetMemoryBlock retrieves a specific memory block.
func (c *Client) GetMemoryBlock(ctx context.Context, agentID, blockID string) (*MemoryBlock, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/agents/"+agentID+"/memory/blocks/"+blockID,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create get memory block request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get memory block request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: get memory block API error %d: %s", resp.StatusCode, string(respBody))
	}

	var block MemoryBlock
	if err := json.NewDecoder(resp.Body).Decode(&block); err != nil {
		return nil, fmt.Errorf("letta: decode get memory block response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &block, nil
}

// UpdateMemoryBlock updates a memory block.
func (c *Client) UpdateMemoryBlock(ctx context.Context, agentID, blockID, value string) (*MemoryBlock, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	reqBody := map[string]string{
		"value": value,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("letta: marshal update memory block request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/agents/"+agentID+"/memory/blocks/"+blockID,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create update memory block request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: update memory block request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: update memory block API error %d: %s", resp.StatusCode, string(respBody))
	}

	var block MemoryBlock
	if err := json.NewDecoder(resp.Body).Decode(&block); err != nil {
		return nil, fmt.Errorf("letta: decode update memory block response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &block, nil
}

// ==================== Source Operations ====================

// CreateSource creates a new data source for RAG.
func (c *Client) CreateSource(ctx context.Context, name, description, sourceType string, metadata map[string]interface{}) (*Source, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	reqBody := map[string]interface{}{
		"name":        name,
		"description": description,
		"type":        sourceType,
		"metadata":    metadata,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("letta: marshal create source request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/sources",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("letta: create source request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: create source request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return nil, fmt.Errorf("letta: create source API error %d: %s", resp.StatusCode, string(respBody))
	}

	var source Source
	if err := json.NewDecoder(resp.Body).Decode(&source); err != nil {
		return nil, fmt.Errorf("letta: decode create source response: %w", err)
	}

	c.breaker.RecordSuccess()
	return &source, nil
}

// UploadFileToSource uploads a file to a source for RAG.
func (c *Client) UploadFileToSource(ctx context.Context, sourceID string, filename string, content []byte) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	// Letta uses multipart form for file uploads
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("letta: create form file: %w", err)
	}
	
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("letta: write file content: %w", err)
	}
	
	if err := writer.Close(); err != nil {
		return fmt.Errorf("letta: close writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/sources/"+sourceID+"/files",
		&buf,
	)
	if err != nil {
		return fmt.Errorf("letta: create upload file request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: upload file request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: upload file API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// AttachSourceToAgent attaches a source to an agent for RAG.
func (c *Client) AttachSourceToAgent(ctx context.Context, agentID, sourceID string) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	reqBody := map[string]string{
		"source_id": sourceID,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("letta: marshal attach source request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint+"/v1/agents/"+agentID+"/sources",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("letta: create attach source request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: attach source request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		c.breaker.RecordFailure()
		return fmt.Errorf("letta: attach source API error %d: %s", resp.StatusCode, string(respBody))
	}

	c.breaker.RecordSuccess()
	return nil
}

// ==================== Core Memory Interface Implementation ====================

// Add stores content as a memory block in Letta.
func (c *Client) Add(ctx context.Context, entry *types.MemoryEntry) error {
	if !c.breaker.Allow() {
		return fmt.Errorf("letta: circuit breaker open")
	}

	// Use agent_id from entry as the target agent
	agentID := entry.AgentID
	if agentID == "" {
		return fmt.Errorf("letta: agent_id is required for adding memory")
	}

	// Create or update a memory block
	blockName := "helix_memory"
	if name, ok := entry.Metadata["block_name"].(string); ok {
		blockName = name
	}

	_, err := c.UpdateMemoryBlock(ctx, agentID, blockName, entry.Content)
	if err != nil {
		return fmt.Errorf("letta: add memory failed: %w", err)
	}

	return nil
}

// Get retrieves a memory by ID (uses memory block ID).
func (c *Client) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	// Letta doesn't have a direct "get memory by ID" - we use memory blocks
	return nil, fmt.Errorf("letta: Get by ID not supported directly, use GetMemoryBlock with agent_id")
}

// Update modifies a memory.
func (c *Client) Update(ctx context.Context, entry *types.MemoryEntry) error {
	return c.Add(ctx, entry) // Same as add for Letta
}

// Delete removes a memory.
func (c *Client) Delete(ctx context.Context, id string) error {
	// Letta doesn't support direct memory deletion - memory blocks are updated
	return fmt.Errorf("letta: Delete not supported directly, use UpdateMemoryBlock to clear")
}

// Search finds memories matching the query (uses agent messages).
func (c *Client) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	start := time.Now()

	agentID := req.AgentID
	if agentID == "" {
		return nil, fmt.Errorf("letta: agent_id is required for search")
	}

	// Send the query as a message and get the response
	// This leverages Letta's built-in retrieval from memory blocks
	resp, err := c.SendMessage(ctx, agentID, req.Query)
	if err != nil {
		return nil, err
	}

	// Create a single entry from the response
	entry := &types.MemoryEntry{
		ID:        uuid.New().String(),
		Content:   resp.Content,
		AgentID:   agentID,
		Type:      types.MemoryTypeProcedural,
		Source:    types.SourceLetta,
		Relevance: 1.0,
		Metadata: map[string]interface{}{
			"response_id": resp.ID,
			"query":       req.Query,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return &types.SearchResult{
		Entries:  []*types.MemoryEntry{entry},
		Total:    1,
		Duration: time.Since(start),
		Sources:  []types.MemorySource{types.SourceLetta},
	}, nil
}

// GetHistory retrieves conversation history for an agent.
func (c *Client) GetHistory(ctx context.Context, agentID string, limit int) ([]*types.MemoryEntry, error) {
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("letta: circuit breaker open")
	}

	messages, err := c.GetMessages(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Convert messages to entries
	entries := make([]*types.MemoryEntry, 0, len(messages))
	for _, msg := range messages {
		entries = append(entries, &types.MemoryEntry{
			ID:        msg.ID,
			Content:   msg.Content,
			AgentID:   agentID,
			Type:      types.MemoryTypeEpisodic,
			Source:    types.SourceLetta,
			Metadata:  map[string]interface{}{"role": msg.Role},
			CreatedAt: msg.CreatedAt,
		})
	}

	// Apply limit
	if limit > 0 && limit < len(entries) {
		entries = entries[len(entries)-limit:]
	}

	return entries, nil
}

// Health checks if Letta is available.
func (c *Client) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.endpoint+"/v1/health",
		nil,
	)
	if err != nil {
		return fmt.Errorf("letta: create health request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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
