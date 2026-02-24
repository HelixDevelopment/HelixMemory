// Package routing implements intelligent memory routing for HelixMemory.
// It classifies incoming memory operations and routes them to the appropriate
// backend(s) based on memory type, content analysis, and configuration.
package routing

import (
	"strings"

	"digital.vasic.helixmemory/pkg/types"
)

// Router classifies and routes memory operations to appropriate backends.
type Router struct {
	// Source priority for write routing
	writePriority map[types.MemoryType]types.MemorySource
	// Sources enabled for read operations
	readSources []types.MemorySource
}

// NewRouter creates a router with default routing rules.
func NewRouter() *Router {
	return &Router{
		writePriority: map[types.MemoryType]types.MemorySource{
			types.MemoryTypeFact:       types.SourceMem0,
			types.MemoryTypeGraph:      types.SourceCognee,
			types.MemoryTypeCore:       types.SourceLetta,
			types.MemoryTypeTemporal:   types.SourceGraphiti,
			types.MemoryTypeEpisodic:   types.SourceLetta,
			types.MemoryTypeProcedural: types.SourceCognee,
		},
		readSources: []types.MemorySource{
			types.SourceMem0,
			types.SourceCognee,
			types.SourceLetta,
			types.SourceGraphiti,
		},
	}
}

// ClassifyMemoryType analyzes content and determines the appropriate memory type.
func (r *Router) ClassifyMemoryType(content string) types.MemoryType {
	lower := strings.ToLower(content)

	// Temporal indicators
	temporalKeywords := []string{
		"yesterday", "last week", "last month", "ago", "since",
		"before", "after", "when", "timeline", "history",
		"changed from", "used to", "previously",
	}
	for _, kw := range temporalKeywords {
		if strings.Contains(lower, kw) {
			return types.MemoryTypeTemporal
		}
	}

	// Procedural indicators
	proceduralKeywords := []string{
		"how to", "step by step", "procedure", "workflow",
		"process", "instructions", "tutorial", "recipe",
		"to do this", "first you", "then you",
	}
	for _, kw := range proceduralKeywords {
		if strings.Contains(lower, kw) {
			return types.MemoryTypeProcedural
		}
	}

	// Graph/relationship indicators
	graphKeywords := []string{
		"relates to", "connected to", "depends on", "part of",
		"belongs to", "is a", "has a", "contains", "implements",
		"extends", "uses", "imports", "calls",
	}
	for _, kw := range graphKeywords {
		if strings.Contains(lower, kw) {
			return types.MemoryTypeGraph
		}
	}

	// Core/persona indicators
	coreKeywords := []string{
		"i am", "my name", "i prefer", "i like", "i dislike",
		"my role", "my job", "i work", "persona", "identity",
	}
	for _, kw := range coreKeywords {
		if strings.Contains(lower, kw) {
			return types.MemoryTypeCore
		}
	}

	// Episodic indicators
	episodicKeywords := []string{
		"conversation", "discussed", "talked about", "mentioned",
		"asked about", "session", "meeting",
	}
	for _, kw := range episodicKeywords {
		if strings.Contains(lower, kw) {
			return types.MemoryTypeEpisodic
		}
	}

	// Default to fact
	return types.MemoryTypeFact
}

// RouteWrite determines which backend should handle a write operation.
func (r *Router) RouteWrite(entry *types.MemoryEntry) types.MemorySource {
	if entry.Type == "" {
		entry.Type = r.ClassifyMemoryType(entry.Content)
	}

	if source, ok := r.writePriority[entry.Type]; ok {
		return source
	}

	return types.SourceMem0
}

// RouteRead determines which backends should be queried for a search.
// Returns all enabled sources if no specific sources are requested.
func (r *Router) RouteRead(req *types.SearchRequest) []types.MemorySource {
	if len(req.Sources) > 0 {
		return req.Sources
	}

	// If specific types are requested, route to primary backends
	if len(req.Types) > 0 {
		sourceSet := make(map[types.MemorySource]struct{})
		for _, t := range req.Types {
			if source, ok := r.writePriority[t]; ok {
				sourceSet[source] = struct{}{}
			}
		}
		// Always include all sources for comprehensive search
		for _, s := range r.readSources {
			sourceSet[s] = struct{}{}
		}
		sources := make([]types.MemorySource, 0, len(sourceSet))
		for s := range sourceSet {
			sources = append(sources, s)
		}
		return sources
	}

	return r.readSources
}

// SetWritePriority overrides the write routing for a memory type.
func (r *Router) SetWritePriority(memType types.MemoryType, source types.MemorySource) {
	r.writePriority[memType] = source
}

// SetReadSources overrides the read sources.
func (r *Router) SetReadSources(sources []types.MemorySource) {
	r.readSources = sources
}
