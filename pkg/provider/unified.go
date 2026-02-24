// Package provider implements the UnifiedMemoryProvider — the nervous system
// of HelixMemory. It orchestrates Mem0, Cognee, Letta, and Graphiti backends
// with parallel search, fusion engine, intelligent routing, and graceful degradation.
package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	"digital.vasic.helixmemory/pkg/config"
	"digital.vasic.helixmemory/pkg/fusion"
	"digital.vasic.helixmemory/pkg/routing"
	"digital.vasic.helixmemory/pkg/types"

	"golang.org/x/sync/errgroup"
)

// UnifiedMemoryProvider orchestrates all memory backends through a single
// interface. It performs parallel search across all available providers,
// fuses results, and degrades gracefully when backends are unavailable.
type UnifiedMemoryProvider struct {
	mu        sync.RWMutex
	providers map[types.MemorySource]types.MemoryProvider
	fusion    *fusion.Engine
	router    *routing.Router
	cfg       *config.Config
}

// New creates a UnifiedMemoryProvider with the given configuration.
func New(cfg *config.Config) *UnifiedMemoryProvider {
	return &UnifiedMemoryProvider{
		providers: make(map[types.MemorySource]types.MemoryProvider),
		fusion:    fusion.NewEngine(cfg),
		router:    routing.NewRouter(),
		cfg:       cfg,
	}
}

// RegisterProvider adds a backend provider to the unified system.
func (u *UnifiedMemoryProvider) RegisterProvider(p types.MemoryProvider) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.providers[p.Name()] = p
}

// Name returns the unified provider name.
func (u *UnifiedMemoryProvider) Name() types.MemorySource {
	return types.SourceFusion
}

// Add stores a memory, routing to the appropriate backend based on type.
func (u *UnifiedMemoryProvider) Add(ctx context.Context, entry *types.MemoryEntry) error {
	target := u.router.RouteWrite(entry)

	u.mu.RLock()
	provider, ok := u.providers[target]
	u.mu.RUnlock()

	if !ok {
		// Fallback to first available provider
		u.mu.RLock()
		for _, p := range u.providers {
			provider = p
			break
		}
		u.mu.RUnlock()
	}

	if provider == nil {
		return fmt.Errorf("helixmemory: no providers available for write")
	}

	if err := provider.Add(ctx, entry); err != nil {
		// Try fallback providers
		u.mu.RLock()
		defer u.mu.RUnlock()
		for source, p := range u.providers {
			if source == target {
				continue
			}
			if fallbackErr := p.Add(ctx, entry); fallbackErr == nil {
				return nil
			}
		}
		return fmt.Errorf("helixmemory: all providers failed to add: %w", err)
	}

	return nil
}

// Search queries all available backends in parallel, fuses results.
func (u *UnifiedMemoryProvider) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	sources := u.router.RouteRead(req)

	u.mu.RLock()
	availableProviders := make([]types.MemoryProvider, 0)
	for _, source := range sources {
		if p, ok := u.providers[source]; ok {
			availableProviders = append(availableProviders, p)
		}
	}
	u.mu.RUnlock()

	if len(availableProviders) == 0 {
		return &types.SearchResult{
			Entries: []*types.MemoryEntry{},
			Total:   0,
		}, nil
	}

	// Parallel search across all available backends
	results := make([]*types.SearchResult, len(availableProviders))
	var resultMu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	if u.cfg.MaxConcurrentQueries > 0 {
		g.SetLimit(u.cfg.MaxConcurrentQueries)
	}

	for i, p := range availableProviders {
		g.Go(func() error {
			searchCtx, cancel := context.WithTimeout(gCtx, u.cfg.RequestTimeout)
			defer cancel()

			result, err := p.Search(searchCtx, req)
			if err != nil {
				// Graceful degradation: log but don't fail
				return nil
			}

			resultMu.Lock()
			results[i] = result
			resultMu.Unlock()
			return nil
		})
	}

	// Wait for all searches (errors are gracefully handled)
	_ = g.Wait()

	// Fuse results from all backends
	return u.fusion.Fuse(results, req), nil
}

// Get retrieves a memory by ID, trying all providers.
func (u *UnifiedMemoryProvider) Get(ctx context.Context, id string) (*types.MemoryEntry, error) {
	u.mu.RLock()
	providers := make([]types.MemoryProvider, 0, len(u.providers))
	for _, p := range u.providers {
		providers = append(providers, p)
	}
	u.mu.RUnlock()

	for _, p := range providers {
		entry, err := p.Get(ctx, id)
		if err == nil && entry != nil {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("helixmemory: memory %s not found in any provider", id)
}

// Update modifies a memory, routing to the owning backend.
func (u *UnifiedMemoryProvider) Update(ctx context.Context, entry *types.MemoryEntry) error {
	u.mu.RLock()

	// Try the source provider first
	if entry.Source != "" {
		if p, ok := u.providers[entry.Source]; ok {
			u.mu.RUnlock()
			return p.Update(ctx, entry)
		}
	}

	// Try all providers
	providers := make([]types.MemoryProvider, 0, len(u.providers))
	for _, p := range u.providers {
		providers = append(providers, p)
	}
	u.mu.RUnlock()

	for _, p := range providers {
		if err := p.Update(ctx, entry); err == nil {
			return nil
		}
	}

	return fmt.Errorf("helixmemory: failed to update memory %s in any provider", entry.ID)
}

// Delete removes a memory, trying all providers.
func (u *UnifiedMemoryProvider) Delete(ctx context.Context, id string) error {
	u.mu.RLock()
	providers := make([]types.MemoryProvider, 0, len(u.providers))
	for _, p := range u.providers {
		providers = append(providers, p)
	}
	u.mu.RUnlock()

	var lastErr error
	for _, p := range providers {
		if err := p.Delete(ctx, id); err != nil {
			lastErr = err
		}
	}

	// If at least one provider succeeded (no error), consider it successful
	if lastErr != nil {
		return fmt.Errorf("helixmemory: delete failed: %w", lastErr)
	}
	return nil
}

// GetHistory returns memory history from all backends, merged.
func (u *UnifiedMemoryProvider) GetHistory(ctx context.Context, userID string, limit int) ([]*types.MemoryEntry, error) {
	u.mu.RLock()
	providers := make([]types.MemoryProvider, 0, len(u.providers))
	for _, p := range u.providers {
		providers = append(providers, p)
	}
	u.mu.RUnlock()

	var allEntries []*types.MemoryEntry
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	for _, p := range providers {
		g.Go(func() error {
			entries, err := p.GetHistory(gCtx, userID, limit)
			if err != nil {
				return nil // Graceful degradation
			}
			mu.Lock()
			allEntries = append(allEntries, entries...)
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()

	// Sort by creation time, newest first
	for i := 0; i < len(allEntries); i++ {
		for j := i + 1; j < len(allEntries); j++ {
			if allEntries[j].CreatedAt.After(allEntries[i].CreatedAt) {
				allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
			}
		}
	}

	if limit > 0 && len(allEntries) > limit {
		allEntries = allEntries[:limit]
	}

	return allEntries, nil
}

// Health checks all registered providers and returns aggregate health.
func (u *UnifiedMemoryProvider) Health(ctx context.Context) error {
	u.mu.RLock()
	providers := make(map[types.MemorySource]types.MemoryProvider, len(u.providers))
	for k, v := range u.providers {
		providers[k] = v
	}
	u.mu.RUnlock()

	if len(providers) == 0 {
		return fmt.Errorf("helixmemory: no providers registered")
	}

	var healthy int
	for _, p := range providers {
		if err := p.Health(ctx); err == nil {
			healthy++
		}
	}

	if healthy == 0 {
		return fmt.Errorf("helixmemory: all %d providers unhealthy", len(providers))
	}

	return nil
}

// HealthDetailed returns health status for each registered provider.
func (u *UnifiedMemoryProvider) HealthDetailed(ctx context.Context) map[types.MemorySource]error {
	u.mu.RLock()
	providers := make(map[types.MemorySource]types.MemoryProvider, len(u.providers))
	for k, v := range u.providers {
		providers[k] = v
	}
	u.mu.RUnlock()

	result := make(map[types.MemorySource]error, len(providers))
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	for source, p := range providers {
		g.Go(func() error {
			err := p.Health(gCtx)
			mu.Lock()
			result[source] = err
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()
	return result
}

// GetProvider returns a specific registered provider.
func (u *UnifiedMemoryProvider) GetProvider(source types.MemorySource) (types.MemoryProvider, bool) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	p, ok := u.providers[source]
	return p, ok
}

// AvailableProviders returns all registered provider sources.
func (u *UnifiedMemoryProvider) AvailableProviders() []types.MemorySource {
	u.mu.RLock()
	defer u.mu.RUnlock()

	sources := make([]types.MemorySource, 0, len(u.providers))
	for s := range u.providers {
		sources = append(sources, s)
	}
	return sources
}

// GetCoreMemory delegates to the Letta provider for core memory blocks.
func (u *UnifiedMemoryProvider) GetCoreMemory(ctx context.Context, agentID string) ([]*types.CoreMemoryBlock, error) {
	u.mu.RLock()
	p, ok := u.providers[types.SourceLetta]
	u.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("helixmemory: Letta provider not registered for core memory")
	}

	if cmp, ok := p.(types.CoreMemoryProvider); ok {
		return cmp.GetCoreMemory(ctx, agentID)
	}

	return nil, fmt.Errorf("helixmemory: Letta provider does not support core memory")
}

// UpdateCoreMemory delegates to the Letta provider.
func (u *UnifiedMemoryProvider) UpdateCoreMemory(ctx context.Context, agentID string, block *types.CoreMemoryBlock) error {
	u.mu.RLock()
	p, ok := u.providers[types.SourceLetta]
	u.mu.RUnlock()

	if !ok {
		return fmt.Errorf("helixmemory: Letta provider not registered for core memory")
	}

	if cmp, ok := p.(types.CoreMemoryProvider); ok {
		return cmp.UpdateCoreMemory(ctx, agentID, block)
	}

	return fmt.Errorf("helixmemory: Letta provider does not support core memory")
}

// TriggerConsolidation starts sleep-time compute across backends.
func (u *UnifiedMemoryProvider) TriggerConsolidation(ctx context.Context, userID string) error {
	u.mu.RLock()
	providers := make([]types.MemoryProvider, 0, len(u.providers))
	for _, p := range u.providers {
		providers = append(providers, p)
	}
	u.mu.RUnlock()

	for _, p := range providers {
		if cp, ok := p.(types.ConsolidationProvider); ok {
			if err := cp.TriggerConsolidation(ctx, userID); err != nil {
				// Log but continue with other providers
				continue
			}
		}
	}

	return nil
}

// SearchTemporal delegates to the Graphiti provider for time-aware queries.
func (u *UnifiedMemoryProvider) SearchTemporal(ctx context.Context, query string, at time.Time) ([]*types.MemoryEntry, error) {
	u.mu.RLock()
	p, ok := u.providers[types.SourceGraphiti]
	u.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("helixmemory: Graphiti provider not registered for temporal search")
	}

	if tp, ok := p.(types.TemporalProvider); ok {
		return tp.SearchTemporal(ctx, query, at)
	}

	return nil, fmt.Errorf("helixmemory: Graphiti provider does not support temporal search")
}
