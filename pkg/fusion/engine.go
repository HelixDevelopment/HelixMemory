// Package fusion provides the unified memory fusion engine for HelixMemory.
// It combines Cognee (knowledge graphs), Mem0 (semantic memory), and Letta (agent memory)
// into a single powerful memory system with intelligent routing and consolidation.
package fusion

import (
	"context"
	"fmt"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/clients/cognee"
	"digital.vasic.helixmemory/pkg/clients/letta"
	"digital.vasic.helixmemory/pkg/clients/mem0"
	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/types"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FusionEngine combines Cognee, Mem0, and Letta into a unified memory system.
type FusionEngine struct {
	cognee    *cognee.Client
	mem0      *mem0.Client
	letta     *letta.Client
	router    *Router
	consolidator *Consolidator
	logger    *zap.Logger
	config    *config.Config

	// Circuit breakers for each provider
	cogneeHealthy bool
	mem0Healthy   bool
	lettaHealthy  bool
	healthMutex   sync.RWMutex
}

// NewFusionEngine creates a new unified memory fusion engine.
func NewFusionEngine(cfg *config.Config, logger *zap.Logger) (*FusionEngine, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	engine := &FusionEngine{
		config:        cfg,
		logger:        logger,
		cogneeHealthy: true,
		mem0Healthy:   true,
		lettaHealthy:  true,
	}

	// Initialize Cognee client
	if cfg.CogneeEndpoint != "" && cfg.CogneeAPIKey != "" {
		engine.cognee = cognee.NewClient(cfg)
		logger.Info("Cognee client initialized")
	}

	// Initialize Mem0 client
	if cfg.Mem0Endpoint != "" && cfg.Mem0APIKey != "" {
		engine.mem0 = mem0.NewClient(cfg)
		logger.Info("Mem0 client initialized")
	}

	// Initialize Letta client
	if cfg.LettaEndpoint != "" && cfg.LettaAPIKey != "" {
		engine.letta = letta.NewClient(cfg)
		logger.Info("Letta client initialized")
	}

	// Initialize router
	engine.router = NewRouter(cfg, logger)

	// Initialize consolidator
	engine.consolidator = NewConsolidator(cfg, logger)

	return engine, nil
}

// ==================== Core Memory Operations ====================

// Store saves a memory entry to the appropriate memory system(s).
func (e *FusionEngine) Store(ctx context.Context, entry *types.MemoryEntry) error {
	start := time.Now()

	// Determine which systems to use based on memory type and routing rules
	systems := e.router.SelectSystemsForWrite(entry)

	var wg sync.WaitGroup
	errChan := make(chan error, len(systems))

	for _, system := range systems {
		wg.Add(1)
		go func(sys types.MemorySource) {
			defer wg.Done()
			if err := e.storeToSystem(ctx, entry, sys); err != nil {
				errChan <- fmt.Errorf("%s: %w", sys, err)
			}
		}(system)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	duration := time.Since(start)
	e.logger.Debug("Memory stored",
		zap.String("id", entry.ID),
		zap.Duration("duration", duration),
		zap.Int("systems", len(systems)),
		zap.Int("errors", len(errors)),
	)

	if len(errors) > 0 && len(errors) == len(systems) {
		return fmt.Errorf("fusion: all systems failed: %v", errors)
	}

	return nil
}

// Retrieve finds memories matching the query across all systems.
func (e *FusionEngine) Retrieve(ctx context.Context, req *types.SearchRequest) (*types.FusionResult, error) {
	start := time.Now()

	// Determine which systems to query
	systems := e.router.SelectSystemsForRead(req)

	// Query all selected systems concurrently
	results := make(map[types.MemorySource]*types.SearchResult)
	var wg sync.WaitGroup
	resultChan := make(chan struct {
		system types.MemorySource
		result *types.SearchResult
		err    error
	}, len(systems))

	for _, system := range systems {
		wg.Add(1)
		go func(sys types.MemorySource) {
			defer wg.Done()
			result, err := e.retrieveFromSystem(ctx, req, sys)
			resultChan <- struct {
				system types.MemorySource
				result *types.SearchResult
				err    error
			}{sys, result, err}
		}(system)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for r := range resultChan {
		if r.err != nil {
			e.logger.Warn("Retrieval from system failed",
				zap.String("system", string(r.system)),
				zap.Error(r.err),
			)
			continue
		}
		results[r.system] = r.result
	}

	// Fuse results
	fusedResult := e.consolidator.FuseResults(results, req)
	fusedResult.Duration = time.Since(start)
	fusedResult.Query = req.Query

	e.logger.Debug("Memory retrieved",
		zap.String("query", req.Query),
		zap.Int("systems_queried", len(systems)),
		zap.Int("systems_responded", len(results)),
		zap.Int("total_results", fusedResult.Total),
		zap.Duration("duration", fusedResult.Duration),
	)

	return fusedResult, nil
}

// Delete removes a memory from all systems.
func (e *FusionEngine) Delete(ctx context.Context, id string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// Delete from all available systems
	if e.cognee != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.cognee.Delete(ctx, id); err != nil {
				errChan <- fmt.Errorf("cognee: %w", err)
			}
		}()
	}

	if e.mem0 != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.mem0.Delete(ctx, id); err != nil {
				errChan <- fmt.Errorf("mem0: %w", err)
			}
		}()
	}

	// Letta doesn't support direct deletion

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("fusion: delete failed: %v", errors)
	}

	return nil
}

// GetHistory retrieves memory history for a user across all systems.
func (e *FusionEngine) GetHistory(ctx context.Context, userID string, limit int) (*types.FusionResult, error) {
	results := make(map[types.MemorySource]*types.SearchResult)

	// Get history from Mem0 (primary user memory store)
	if e.mem0 != nil && e.isHealthy(types.SourceMem0) {
		entries, err := e.mem0.GetHistory(ctx, userID, limit)
		if err == nil {
			results[types.SourceMem0] = &types.SearchResult{
				Entries: entries,
				Total:   len(entries),
				Sources: []types.MemorySource{types.SourceMem0},
			}
		}
	}

	// Get from Cognee if available
	if e.cognee != nil && e.isHealthy(types.SourceCognee) {
		entries, err := e.cognee.GetHistory(ctx, userID, limit)
		if err == nil {
			results[types.SourceCognee] = &types.SearchResult{
				Entries: entries,
				Total:   len(entries),
				Sources: []types.MemorySource{types.SourceCognee},
			}
		}
	}

	// Fuse results
	fusedResult := e.consolidator.FuseResults(results, &types.SearchRequest{
		UserID: userID,
		TopK:   limit,
	})

	return fusedResult, nil
}

// ==================== System-Specific Operations ====================

func (e *FusionEngine) storeToSystem(ctx context.Context, entry *types.MemoryEntry, system types.MemorySource) error {
	switch system {
	case types.SourceCognee:
		if e.cognee == nil || !e.isHealthy(system) {
			return fmt.Errorf("cognee not available")
		}
		return e.cognee.Add(ctx, entry)

	case types.SourceMem0:
		if e.mem0 == nil || !e.isHealthy(system) {
			return fmt.Errorf("mem0 not available")
		}
		return e.mem0.Add(ctx, entry)

	case types.SourceLetta:
		if e.letta == nil || !e.isHealthy(system) {
			return fmt.Errorf("letta not available")
		}
		return e.letta.Add(ctx, entry)

	default:
		return fmt.Errorf("unknown memory system: %s", system)
	}
}

func (e *FusionEngine) retrieveFromSystem(ctx context.Context, req *types.SearchRequest, system types.MemorySource) (*types.SearchResult, error) {
	switch system {
	case types.SourceCognee:
		if e.cognee == nil || !e.isHealthy(system) {
			return nil, fmt.Errorf("cognee not available")
		}
		return e.cognee.Search(ctx, req)

	case types.SourceMem0:
		if e.mem0 == nil || !e.isHealthy(system) {
			return nil, fmt.Errorf("mem0 not available")
		}
		return e.mem0.Search(ctx, req)

	case types.SourceLetta:
		if e.letta == nil || !e.isHealthy(system) {
			return nil, fmt.Errorf("letta not available")
		}
		return e.letta.Search(ctx, req)

	default:
		return nil, fmt.Errorf("unknown memory system: %s", system)
	}
}

// ==================== Health Management ====================

// HealthCheck performs health checks on all memory systems.
func (e *FusionEngine) HealthCheck(ctx context.Context) map[types.MemorySource]error {
	results := make(map[types.MemorySource]error)

	// Check Cognee
	if e.cognee != nil {
		err := e.cognee.Health(ctx)
		results[types.SourceCognee] = err
		e.setHealth(types.SourceCognee, err == nil)
	}

	// Check Mem0
	if e.mem0 != nil {
		err := e.mem0.Health(ctx)
		results[types.SourceMem0] = err
		e.setHealth(types.SourceMem0, err == nil)
	}

	// Check Letta
	if e.letta != nil {
		err := e.letta.Health(ctx)
		results[types.SourceLetta] = err
		e.setHealth(types.SourceLetta, err == nil)
	}

	return results
}

func (e *FusionEngine) isHealthy(system types.MemorySource) bool {
	e.healthMutex.RLock()
	defer e.healthMutex.RUnlock()

	switch system {
	case types.SourceCognee:
		return e.cogneeHealthy
	case types.SourceMem0:
		return e.mem0Healthy
	case types.SourceLetta:
		return e.lettaHealthy
	default:
		return false
	}
}

func (e *FusionEngine) setHealth(system types.MemorySource, healthy bool) {
	e.healthMutex.Lock()
	defer e.healthMutex.Unlock()

	switch system {
	case types.SourceCognee:
		e.cogneeHealthy = healthy
	case types.SourceMem0:
		e.mem0Healthy = healthy
	case types.SourceLetta:
		e.lettaHealthy = healthy
	}
}

// ==================== Advanced Operations ====================

// Consolidate triggers memory consolidation across all systems.
func (e *FusionEngine) Consolidate(ctx context.Context) error {
	return e.consolidator.RunConsolidation(ctx)
}

// GetStats returns statistics for all memory systems.
func (e *FusionEngine) GetStats() types.FusionStats {
	return types.FusionStats{
		CogneeHealthy: e.isHealthy(types.SourceCognee),
		Mem0Healthy:   e.isHealthy(types.SourceMem0),
		LettaHealthy:  e.isHealthy(types.SourceLetta),
		Timestamp:     time.Now(),
	}
}

// Query performs a natural language query across all memory systems.
func (e *FusionEngine) Query(ctx context.Context, query string, userID string) (*types.FusionResult, error) {
	req := &types.SearchRequest{
		Query:  query,
		UserID: userID,
		TopK:   10,
	}
	return e.Retrieve(ctx, req)
}

// StoreWithAgent stores a memory and associates it with a Letta agent.
func (e *FusionEngine) StoreWithAgent(ctx context.Context, entry *types.MemoryEntry, agentID string) error {
	// Store in Mem0 for persistence
	if e.mem0 != nil {
		entry.AgentID = agentID
		if err := e.mem0.Add(ctx, entry); err != nil {
			e.logger.Warn("Failed to store in Mem0", zap.Error(err))
		}
	}

	// Store in Letta for agent context
	if e.letta != nil {
		entry.AgentID = agentID
		if err := e.letta.Add(ctx, entry); err != nil {
			e.logger.Warn("Failed to store in Letta", zap.Error(err))
		}
	}

	return nil
}

// RetrieveForAgent retrieves memories for a specific agent.
func (e *FusionEngine) RetrieveForAgent(ctx context.Context, query, agentID string) (*types.FusionResult, error) {
	req := &types.SearchRequest{
		Query:   query,
		AgentID: agentID,
		TopK:    10,
	}

	// Prioritize Letta for agent-specific queries
	results := make(map[types.MemorySource]*types.SearchResult)

	if e.letta != nil && e.isHealthy(types.SourceLetta) {
		result, err := e.letta.Search(ctx, req)
		if err == nil {
			results[types.SourceLetta] = result
		}
	}

	// Also check Mem0 for broader context
	if e.mem0 != nil && e.isHealthy(types.SourceMem0) {
		result, err := e.mem0.Search(ctx, req)
		if err == nil {
			results[types.SourceMem0] = result
		}
	}

	return e.consolidator.FuseResults(results, req), nil
}

// CreateKnowledgeGraph creates a knowledge graph entry in Cognee.
func (e *FusionEngine) CreateKnowledgeGraph(ctx context.Context, content string, metadata map[string]interface{}) error {
	if e.cognee == nil || !e.isHealthy(types.SourceCognee) {
		return fmt.Errorf("cognee not available")
	}

	entry := &types.MemoryEntry{
		ID:        uuid.New().String(),
		Content:   content,
		Type:      types.MemoryTypeGraph,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return e.cognee.Add(ctx, entry)
}

// QueryKnowledgeGraph queries the knowledge graph in Cognee.
func (e *FusionEngine) QueryKnowledgeGraph(ctx context.Context, query string) (*types.SearchResult, error) {
	if e.cognee == nil || !e.isHealthy(types.SourceCognee) {
		return nil, fmt.Errorf("cognee not available")
	}

	req := &types.SearchRequest{
		Query:  query,
		TopK:   10,
		Filter: map[string]interface{}{"search_type": "GRAPH_COMPLETION"},
	}

	return e.cognee.Search(ctx, req)
}

// Fuse merges search results from multiple backends into a single result
// using the consolidator's deduplication and re-ranking pipeline.
func (e *FusionEngine) Fuse(results []*types.SearchResult, req *types.SearchRequest) *types.SearchResult {
	// Convert slice to map keyed by index for the consolidator
	mapped := make(map[types.MemorySource]*types.SearchResult)
	for i, r := range results {
		if r != nil {
			mapped[types.MemorySource(fmt.Sprintf("source_%d", i))] = r
		}
	}
	fusedResult := e.consolidator.FuseResults(mapped, req)
	if fusedResult == nil {
		return &types.SearchResult{Entries: []*types.MemoryEntry{}, Total: 0}
	}
	return &types.SearchResult{
		Entries: fusedResult.Entries,
		Total:   len(fusedResult.Entries),
	}
}
