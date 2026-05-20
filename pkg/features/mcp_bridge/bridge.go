// Package mcp_bridge implements the MCP (Model Context Protocol) Bridge
// for HelixMemory. It exposes memory operations through MCP-compatible
// endpoints, enabling external tools and agents to interact with memory.
package mcp_bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"digital.vasic.helixmemory/pkg/i18n"
	"digital.vasic.helixmemory/pkg/types"
)

// Tool represents an MCP tool exposed by the memory bridge.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// ToolCall represents an incoming MCP tool call.
type ToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"isError,omitempty"`
}

// Bridge exposes HelixMemory operations as MCP tools.
type Bridge struct {
	provider types.MemoryProvider
}

// NewBridge creates an MCP bridge for memory operations.
func NewBridge(provider types.MemoryProvider) *Bridge {
	return &Bridge{provider: provider}
}

// ListTools returns all available MCP tools with descriptions rendered in the
// active translator's default locale. Equivalent to ListToolsLocalized("").
func (b *Bridge) ListTools() []Tool {
	return b.ListToolsLocalized("")
}

// ListToolsLocalized returns all available MCP tools with every user-facing
// description and parameter helper text rendered for the given BCP-47 locale.
//
// CONST-046: tool descriptions are end-user-facing surfaces (rendered in MCP
// clients) — they MUST NOT be hardcoded English literals. Every string here
// is resolved through the i18n seam against the helixmemory_ bundle so a
// consumer that registers a locale-aware translator (i18n.Set) surfaces
// localised text without any change to this file.
func (b *Bridge) ListToolsLocalized(locale string) []Tool {
	t := func(key string) string { return i18n.T(locale, i18n.BundlePrefix+key) }
	return []Tool{
		{
			Name:        "memory_search",
			Description: t("mcp_tool_search_desc"),
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":   map[string]string{"type": "string", "description": t("mcp_param_query_desc")},
					"top_k":   map[string]interface{}{"type": "integer", "description": t("mcp_param_top_k_desc"), "default": 10},
					"user_id": map[string]string{"type": "string", "description": t("mcp_param_user_filter_desc")},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "memory_add",
			Description: t("mcp_tool_add_desc"),
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]string{"type": "string", "description": t("mcp_param_content_desc")},
					"type":    map[string]string{"type": "string", "description": t("mcp_param_mem_type_desc")},
					"user_id": map[string]string{"type": "string", "description": t("mcp_param_user_id_desc")},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "memory_health",
			Description: t("mcp_tool_health_desc"),
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "memory_get",
			Description: t("mcp_tool_get_desc"),
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]string{"type": "string", "description": t("mcp_param_memory_id_desc")},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "memory_delete",
			Description: t("mcp_tool_delete_desc"),
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]string{"type": "string", "description": t("mcp_param_memory_id_desc")},
				},
				"required": []string{"id"},
			},
		},
	}
}

// HandleToolCall processes an MCP tool call and returns the result.
func (b *Bridge) HandleToolCall(ctx context.Context, call *ToolCall) *ToolResult {
	switch call.Name {
	case "memory_search":
		return b.handleSearch(ctx, call.Input)
	case "memory_add":
		return b.handleAdd(ctx, call.Input)
	case "memory_health":
		return b.handleHealth(ctx)
	case "memory_get":
		return b.handleGet(ctx, call.Input)
	case "memory_delete":
		return b.handleDelete(ctx, call.Input)
	default:
		return &ToolResult{Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}
	}
}

func (b *Bridge) handleSearch(ctx context.Context, input json.RawMessage) *ToolResult {
	var params struct {
		Query  string `json:"query"`
		TopK   int    `json:"top_k"`
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{Content: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	if params.TopK <= 0 {
		params.TopK = 10
	}

	req := &types.SearchRequest{
		Query:  params.Query,
		TopK:   params.TopK,
		UserID: params.UserID,
	}

	result, err := b.provider.Search(ctx, req)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("search error: %v", err), IsError: true}
	}

	data, _ := json.Marshal(result)
	return &ToolResult{Content: string(data)}
}

func (b *Bridge) handleAdd(ctx context.Context, input json.RawMessage) *ToolResult {
	var params struct {
		Content string `json:"content"`
		Type    string `json:"type"`
		UserID  string `json:"user_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{Content: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	memType := types.MemoryTypeFact
	if params.Type != "" {
		memType = types.MemoryType(params.Type)
	}

	entry := &types.MemoryEntry{
		Content:   params.Content,
		Type:      memType,
		UserID:    params.UserID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := b.provider.Add(ctx, entry); err != nil {
		return &ToolResult{Content: fmt.Sprintf("add error: %v", err), IsError: true}
	}

	return &ToolResult{Content: "memory added successfully"}
}

func (b *Bridge) handleHealth(ctx context.Context) *ToolResult {
	if err := b.provider.Health(ctx); err != nil {
		return &ToolResult{Content: fmt.Sprintf("unhealthy: %v", err), IsError: true}
	}
	return &ToolResult{Content: "all memory backends healthy"}
}

func (b *Bridge) handleGet(ctx context.Context, input json.RawMessage) *ToolResult {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{Content: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	entry, err := b.provider.Get(ctx, params.ID)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("get error: %v", err), IsError: true}
	}

	data, _ := json.Marshal(entry)
	return &ToolResult{Content: string(data)}
}

func (b *Bridge) handleDelete(ctx context.Context, input json.RawMessage) *ToolResult {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{Content: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	if err := b.provider.Delete(ctx, params.ID); err != nil {
		return &ToolResult{Content: fmt.Sprintf("delete error: %v", err), IsError: true}
	}

	return &ToolResult{Content: "memory deleted successfully"}
}
