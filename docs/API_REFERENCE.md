# HelixMemory - API Reference

**Module:** `digital.vasic.helixmemory`

## Core Interfaces (`pkg/types`)

### MemoryProvider

Base contract for all memory backends. Implemented by Mem0, Cognee,
Letta, and Graphiti clients.

```go
type MemoryProvider interface {
    Name() string
    Add(ctx context.Context, entry *MemoryEntry) (string, error)
    Search(ctx context.Context, query SearchQuery) (*SearchResult, error)
    Get(ctx context.Context, id string) (*MemoryEntry, error)
    Update(ctx context.Context, entry *MemoryEntry) error
    Delete(ctx context.Context, id string) error
    GetHistory(ctx context.Context, userID string, limit int) ([]*MemoryEntry, error)
    Health(ctx context.Context) error
}
```

### CoreMemoryProvider

Extends `MemoryProvider` for Letta-style editable in-context memory.

```go
type CoreMemoryProvider interface {
    MemoryProvider
    GetCoreMemory(ctx context.Context, agentID string) (map[string]string, error)
    UpdateCoreMemory(ctx context.Context, agentID string, section string, content string) error
}
```

### ConsolidationProvider

Extends `MemoryProvider` with sleep-time compute capabilities.

```go
type ConsolidationProvider interface {
    MemoryProvider
    TriggerConsolidation(ctx context.Context) error
    GetConsolidationStatus(ctx context.Context) (*ConsolidationStatus, error)
}
```

### TemporalProvider

Extends `MemoryProvider` with time-aware query capabilities via Graphiti.

```go
type TemporalProvider interface {
    MemoryProvider
    SearchTemporal(ctx context.Context, query TemporalQuery) (*SearchResult, error)
    GetTimeline(ctx context.Context, entityID string, start, end time.Time) ([]*MemoryEntry, error)
    InvalidateAt(ctx context.Context, entityID string, timestamp time.Time) error
}
```

### InfraProvider

Abstracts container infrastructure for the 7 required services.

```go
type InfraProvider interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    HealthCheck(ctx context.Context) (map[string]bool, error)
    GetEndpoint(service string) string
    IsRemote() bool
}
```

## Core Types

### MemoryEntry

```go
type MemoryEntry struct {
    ID         string                 `json:"id"`
    Content    string                 `json:"content"`
    Type       MemoryType             `json:"type"`
    Source     MemorySource           `json:"source"`
    UserID     string                 `json:"user_id"`
    Confidence float64               `json:"confidence"`
    Relevance  float64               `json:"relevance"`
    Embedding  []float64             `json:"embedding,omitempty"`
    Metadata   map[string]interface{} `json:"metadata,omitempty"`
    CreatedAt  time.Time             `json:"created_at"`
    UpdatedAt  time.Time             `json:"updated_at"`
}
```

### SearchQuery / SearchResult

```go
type SearchQuery struct {
    Query     string
    UserID    string
    Type      MemoryType
    Limit     int
    MinScore  float64
    Metadata  map[string]interface{}
}

type SearchResult struct {
    Entries    []*MemoryEntry
    TotalCount int
    Sources    []string
}
```

### TemporalQuery

```go
type TemporalQuery struct {
    Query    string
    EntityID string
    Start    time.Time
    End      time.Time
    Limit    int
}
```

## Enums

### MemoryType

| Constant | Value | Primary Backend |
|----------|-------|-----------------|
| `MemoryTypeFact` | `"fact"` | Mem0 |
| `MemoryTypeGraph` | `"graph"` | Cognee |
| `MemoryTypeCore` | `"core"` | Letta |
| `MemoryTypeTemporal` | `"temporal"` | Graphiti |
| `MemoryTypeEpisodic` | `"episodic"` | Letta |
| `MemoryTypeProcedural` | `"procedural"` | Cognee |

### MemorySource

| Constant | Value | Trust Score |
|----------|-------|-------------|
| `MemorySourceMem0` | `"mem0"` | 0.85 |
| `MemorySourceCognee` | `"cognee"` | 0.80 |
| `MemorySourceLetta` | `"letta"` | 0.95 |
| `MemorySourceGraphiti` | `"graphiti"` | 0.90 |

## Fusion Engine (`pkg/fusion`)

### FusionEngine

The 3-stage pipeline for combining multi-backend search results.

```go
func NewFusionEngine(config *FusionConfig) *FusionEngine
func (f *FusionEngine) Fuse(results []*SearchResult) *SearchResult
```

**Scoring formula:**

```
score = relevance * 0.40 + recency * 0.25 + source * 0.20 + type * 0.15
```

**Deduplication threshold:** cosine similarity >= 0.92 (configurable).

## Router (`pkg/routing`)

```go
func NewRouter() *Router
func (r *Router) ClassifyType(content string) MemoryType
func (r *Router) RouteWrite(entry *MemoryEntry) string
func (r *Router) RouteRead(query *SearchQuery) []string
```

## UnifiedMemoryProvider (`pkg/provider`)

```go
func NewUnifiedMemoryProvider(config *config.Config) *UnifiedMemoryProvider
func (u *UnifiedMemoryProvider) RegisterProvider(provider MemoryProvider)
func (u *UnifiedMemoryProvider) Add(ctx context.Context, entry *MemoryEntry) (string, error)
func (u *UnifiedMemoryProvider) Search(ctx context.Context, query SearchQuery) (*SearchResult, error)
func (u *UnifiedMemoryProvider) Get(ctx context.Context, id string) (*MemoryEntry, error)
func (u *UnifiedMemoryProvider) Health(ctx context.Context) *HealthStatus
```

## MemoryStoreAdapter (`pkg/provider`)

Bridges HelixMemory to the `digital.vasic.memory/pkg/store.MemoryStore`
interface for drop-in replacement.

```go
func NewMemoryStoreAdapter(provider *UnifiedMemoryProvider) *MemoryStoreAdapter
```

## Consolidation (`pkg/consolidation`)

```go
func NewConsolidationEngine(providers []MemoryProvider, config *config.Config) *ConsolidationEngine
func (c *ConsolidationEngine) Start(ctx context.Context)
func (c *ConsolidationEngine) Stop()
func (c *ConsolidationEngine) GetStatus() *ConsolidationStatus

type ConsolidationStatus struct {
    Runs         int
    Processed    int
    Deduplicated int
    Errors       int
    LastRunAt    time.Time
}
```

## Metrics (`pkg/metrics`)

Prometheus metrics are registered under the `helixmemory_` namespace:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `helixmemory_search_duration_seconds` | Histogram | source, status | Search latency |
| `helixmemory_add_duration_seconds` | Histogram | source, status | Add latency |
| `helixmemory_operations_total` | Counter | operation, source | Operation counts |
| `helixmemory_provider_health` | Gauge | provider | Provider health (1=up, 0=down) |
| `helixmemory_fusion_entries_total` | Counter | stage | Fusion pipeline entry counts |
| `helixmemory_active_providers` | Gauge | -- | Number of active providers |
