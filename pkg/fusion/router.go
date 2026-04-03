// Package fusion provides intelligent routing for the HelixMemory fusion engine.
package fusion

import (
	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"go.uber.org/zap"
)

// Router intelligently routes memory operations to appropriate backends.
type Router struct {
	config *config.Config
	logger *zap.Logger
}

// NewRouter creates a new memory router.
func NewRouter(cfg *config.Config, logger *zap.Logger) *Router {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Router{
		config: cfg,
		logger: logger,
	}
}

// SelectSystemsForWrite determines which memory systems to use for storing a memory.
func (r *Router) SelectSystemsForWrite(entry *types.MemoryEntry) []types.MemorySource {
	systems := make([]types.MemorySource, 0)

	switch entry.Type {
	case types.MemoryTypeGraph:
		// Knowledge graph entries go to Cognee
		systems = append(systems, types.SourceCognee)
		// Also store in Mem0 for semantic search
		systems = append(systems, types.SourceMem0)

	case types.MemoryTypeCore:
		// Core/agent memory goes to Letta
		systems = append(systems, types.SourceLetta)
		// Also backup in Mem0
		systems = append(systems, types.SourceMem0)

	case types.MemoryTypeEpisodic:
		// Episodic memories go to Mem0 and Letta
		systems = append(systems, types.SourceMem0)
		if entry.AgentID != "" {
			systems = append(systems, types.SourceLetta)
		}

	case types.MemoryTypeFact:
		// Facts go to Mem0 and Cognee
		systems = append(systems, types.SourceMem0)
		systems = append(systems, types.SourceCognee)

	case types.MemoryTypeProcedural:
		// Procedural memories go to Letta
		systems = append(systems, types.SourceLetta)

	default:
		// Default: store in Mem0 for general semantic memory
		systems = append(systems, types.SourceMem0)
	}

	// If agent-specific, ensure Letta is included
	if entry.AgentID != "" {
		hasLetta := false
		for _, s := range systems {
			if s == types.SourceLetta {
				hasLetta = true
				break
			}
		}
		if !hasLetta {
			systems = append(systems, types.SourceLetta)
		}
	}

	// Remove duplicates while preserving order
	systems = r.deduplicate(systems)

	r.logger.Debug("Selected systems for write",
		zap.String("type", string(entry.Type)),
		zap.Strings("systems", sourceToStrings(systems)),
	)

	return systems
}

// SelectSystemsForRead determines which memory systems to query.
func (r *Router) SelectSystemsForRead(req *types.SearchRequest) []types.MemorySource {
	systems := make([]types.MemorySource, 0)

	// If specific sources requested, use those
	if len(req.Sources) > 0 {
		return req.Sources
	}

	// If agent-specific query, prioritize Letta
	if req.AgentID != "" {
		systems = append(systems, types.SourceLetta)
		systems = append(systems, types.SourceMem0)
		return systems
	}

	// If types specified, route based on types
	if len(req.Types) > 0 {
		hasGraph := false
		hasCore := false
		hasOther := false

		for _, t := range req.Types {
			switch t {
			case types.MemoryTypeGraph:
				hasGraph = true
			case types.MemoryTypeCore:
				hasCore = true
			default:
				hasOther = true
			}
		}

		if hasGraph {
			systems = append(systems, types.SourceCognee)
		}
		if hasCore {
			systems = append(systems, types.SourceLetta)
		}
		if hasOther {
			systems = append(systems, types.SourceMem0)
		}

		return systems
	}

	// Default: query all available systems
	systems = append(systems, types.SourceMem0)
	systems = append(systems, types.SourceCognee)

	r.logger.Debug("Selected systems for read",
		zap.String("query", req.Query),
		zap.Strings("systems", sourceToStrings(systems)),
	)

	return systems
}

// deduplicate removes duplicate sources while preserving order.
func (r *Router) deduplicate(sources []types.MemorySource) []types.MemorySource {
	seen := make(map[types.MemorySource]bool)
	result := make([]types.MemorySource, 0)

	for _, s := range sources {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

func sourceToStrings(sources []types.MemorySource) []string {
	result := make([]string, len(sources))
	for i, s := range sources {
		result[i] = string(s)
	}
	return result
}
