# HelixMemory Power Features

HelixMemory ships with 12 power features that extend the core memory system with specialized capabilities. Each feature is implemented as an independent package under `pkg/features/` and depends on the `types.MemoryProvider` interface.

---

## 1. Codebase DNA Profiling

**Package:** `pkg/features/codebase_dna`

Builds a memory profile of coding patterns, preferences, architecture decisions, and conventions from analyzing codebase interactions. The DNA profile captures the "genetic code" of a project so that AI agents can generate code that follows established patterns.

### Key Types

- `Profile` -- The complete DNA profile: language, framework, patterns, preferences, conventions, dependencies.
- `Pattern` -- A detected coding pattern with name, description, frequency, and confidence.
- `Preference` -- A coding preference (e.g., test framework: testify, naming: camelCase).
- `Convention` -- A project convention with rule, scope, and enforcement level.

### Key API Methods

```go
profiler := codebase_dna.NewProfiler(memoryProvider)

// Analyze code and store detected patterns as memories
profile, err := profiler.AnalyzeCode(ctx, sourceCode, "go", "HelixAgent")

// Retrieve a previously built DNA profile
profile, err := profiler.GetProfile(ctx, "HelixAgent")
```

### Example

```go
profiler := codebase_dna.NewProfiler(unified)

code := `
package main

import "context"

type Service interface {
    Process(ctx context.Context) error
}
`

profile, _ := profiler.AnalyzeCode(ctx, code, "go", "myproject")
// profile.Patterns: [interface_abstraction, context_propagation]
// profile.Preferences: [{naming, convention, camelCase}]
```

### Detection Capabilities

For Go code, the profiler detects:
- Interface abstraction usage
- Table-driven test patterns
- Context propagation through call chains
- Mutex-based concurrency control
- Error wrapping with `%w`

For Python code: abstract base class usage.

### Dependencies

Requires any MemoryProvider. Stores patterns as `procedural` type memories with `dna_*` metadata keys.

---

## 2. Procedural Memory

**Package:** `pkg/features/procedural`

Captures learned workflows, debugging strategies, deployment procedures, and other "how-to" knowledge. Procedures are multi-step recipes that improve over time based on success/failure feedback.

### Key Types

- `Procedure` -- A named workflow with ordered steps, success rate, and usage count.
- `Step` -- A single step with order, action, description, optional command, and expected output.

### Key API Methods

```go
manager := procedural.NewManager(memoryProvider)

// Learn a new procedure
proc, err := manager.LearnProcedure(ctx, "deploy-to-staging", "Deploy the app to staging", steps)

// Find procedures matching a query
procs, err := manager.FindProcedure(ctx, "how to deploy")

// Record outcome to update success rate
err := manager.RecordOutcome(ctx, procedureID, true)
```

### Example

```go
manager := procedural.NewManager(unified)

steps := []procedural.Step{
    {Order: 1, Action: "Build binary", Command: "make build"},
    {Order: 2, Action: "Run tests", Command: "make test"},
    {Order: 3, Action: "Deploy", Command: "make deploy-staging"},
}

proc, _ := manager.LearnProcedure(ctx, "deploy-staging",
    "Deploy application to staging environment", steps)

// After successful execution
manager.RecordOutcome(ctx, proc.ID, true)
// success_rate updates via exponential moving average
```

### Self-Improving Success Rate

`RecordOutcome` uses an exponential moving average to update the success rate:
- Success: `new_rate = old_rate * 0.9 + 0.1`
- Failure: `new_rate = old_rate * 0.9`

This means recent outcomes have more weight than older ones.

### Dependencies

Requires any MemoryProvider. Stores procedures as `procedural` type memories.

---

## 3. Multi-Agent Memory Mesh

**Package:** `pkg/features/mesh`

Enables multiple AI agents to share memories through a unified mesh with scope isolation and access control. Agents can share knowledge at private, team, or global scope levels.

### Key Types

- `Mesh` -- The mesh controller managing agents and shared memories.
- `AgentInfo` -- An agent's identity: ID, name, role, team.
- `MeshScope` -- Visibility level: `private`, `team`, `global`.

### Key API Methods

```go
m := mesh.NewMesh(memoryProvider)

// Register agents
m.RegisterAgent(&mesh.AgentInfo{ID: "agent-1", Name: "Coder", Team: "dev"})
m.RegisterAgent(&mesh.AgentInfo{ID: "agent-2", Name: "Reviewer", Team: "dev"})

// Share a memory with team visibility
m.ShareMemory(ctx, "agent-1", entry, mesh.ScopeTeam)

// Search for memories visible to an agent
entries, err := m.SearchMeshMemories(ctx, "agent-2", "coding standards", 10)

// Transfer knowledge between agents
transferred, err := m.TransferKnowledge(ctx, "agent-1", "agent-2", "patterns", 5)
```

### Example

```go
m := mesh.NewMesh(unified)

m.RegisterAgent(&mesh.AgentInfo{
    ID: "debate-lead", Name: "Debate Leader", Role: "coordinator", Team: "debate",
})
m.RegisterAgent(&mesh.AgentInfo{
    ID: "debate-critic", Name: "Critic", Role: "reviewer", Team: "debate",
})

// Leader shares a finding with the team
entry := &types.MemoryEntry{Content: "Consensus: use factory pattern for providers"}
m.ShareMemory(ctx, "debate-lead", entry, mesh.ScopeTeam)

// Critic can see it (same team)
results, _ := m.SearchMeshMemories(ctx, "debate-critic", "factory pattern", 5)
```

### Scope Visibility Rules

| Scope   | Who Can See                              |
|---------|------------------------------------------|
| private | Only the agent who created the memory    |
| team    | All agents with the same team identifier |
| global  | All agents in the mesh                   |

### Dependencies

Requires any MemoryProvider. Stores scope metadata in entry's `mesh_scope`, `mesh_owner`, `mesh_team` fields.

---

## 4. Temporal Reasoning

**Package:** `pkg/features/temporal`

Enables bi-temporal queries and timeline construction over memories. Answers questions like "What was true at time T?" and "What changed between T1 and T2?"

### Key Types

- `Reasoner` -- The temporal reasoning engine.
- `TimelineEntry` -- A memory entry with validity interval (`valid_at`, `invalid_at`, `is_active`).
- `Timeline` -- A chronological sequence of timeline entries.

### Key API Methods

```go
reasoner := temporal.NewReasoner(memoryProvider)

// What was true at a specific point in time?
entries, err := reasoner.WhatWasTrue(ctx, "deployment config", specificTime)

// Build a timeline of changes
timeline, err := reasoner.BuildTimeline(ctx, "API schema", startTime, endTime)

// What changed between two points?
changes, err := reasoner.WhatChanged(ctx, "database schema", lastWeek, now)
```

### Example

```go
reasoner := temporal.NewReasoner(unified)

// What did we know about the auth system last month?
lastMonth := time.Now().AddDate(0, -1, 0)
entries, _ := reasoner.WhatWasTrue(ctx, "authentication", lastMonth)

for _, e := range entries {
    status := "active"
    if !e.IsActive {
        status = "superseded"
    }
    fmt.Printf("[%s] %s: %s\n", status, e.ValidAt.Format("2006-01-02"), e.Memory.Content)
}
```

### Bi-Temporal Model

Each temporal entry has two time dimensions:
- **valid_at**: When the fact became true in the real world.
- **invalid_at**: When the fact ceased to be true (nil = still valid).

This allows queries like "show me what was in the API contract on January 15th" even if the memory was recorded earlier or updated later.

### Dependencies

Requires any MemoryProvider (works best with Graphiti as the temporal backend).

---

## 5. Confidence Scoring and Provenance

**Package:** `pkg/features/confidence`

Calculates confidence scores for every memory entry based on five weighted factors. Also provides full provenance tracking so every memory can be traced to its origin.

### Key Types

- `Scorer` -- Calculates confidence scores.
- `ScoreWeights` -- Configurable weights for the 5 scoring components.
- `Provenance` -- Tracks a memory's origin and transformation history.
- `Transformation` -- A recorded change (fusion, consolidation, enrichment).

### Key API Methods

```go
scorer := confidence.NewScorer(nil) // uses default weights

// Score a memory entry
score := scorer.Score(entry, crossValidationCount)

// Create provenance for a memory
prov := confidence.NewProvenance(entry)
prov.AddTransformation("fusion", "merged from mem0 and cognee results")
```

### Confidence Formula

```
confidence = source_reliability * 0.30
           + cross_validation   * 0.25
           + recency_decay      * 0.20
           + access_frequency   * 0.15
           + content_coherence  * 0.10
```

**Source Reliability (30%):** Trust score for each backend (Letta: 0.95, Mem0: 0.85, Graphiti: 0.85, Cognee: 0.80).

**Cross-Validation (25%):** Rewards memories confirmed by multiple sources. 0 validations = 0.3, 1 = 0.533, 2 = 0.766, 3+ = 1.0.

**Recency Decay (20%):** Exponential decay with 7-day half-life: `exp(-0.00413 * hours)`.

**Access Frequency (15%):** Logarithmic scaling: `log10(access_count + 1) / 3.0`. Rewards frequently accessed memories.

**Content Coherence (10%):** Heuristic based on content length. Empty = 0.0, very short (<10 chars) = 0.3, very long (>5000 chars) = 0.7, normal = 0.8.

### Dependencies

Self-contained. Only requires a `types.MemoryEntry` for scoring.

---

## 6. Self-Improving Memory Quality Loop

**Package:** `pkg/features/quality_loop`

Continuously monitors memory quality, identifies stale, contradictory, or low-confidence entries, and generates recommended improvement actions.

### Key Types

- `Loop` -- The quality loop controller (periodic background task).
- `QualityReport` -- Results of a quality scan: counts, averages, recommended actions.
- `Action` -- A recommended improvement (prune, refresh, merge, validate).
- `Config` -- Loop configuration: interval, thresholds, batch size.

### Key API Methods

```go
loop := quality_loop.NewLoop(memoryProvider, quality_loop.DefaultConfig())

// Run a one-time quality analysis
report, err := loop.Analyze(ctx)

// Start the periodic loop
loop.Start(ctx)
defer loop.Stop()

// Check loop statistics
stats := loop.GetStats()
```

### Example

```go
loop := quality_loop.NewLoop(unified, quality_loop.Config{
    Enabled:            true,
    Interval:           1 * time.Hour,
    StaleThreshold:     30 * 24 * time.Hour,
    LowConfidenceLimit: 0.3,
    MaxMemoriesPerScan: 500,
})

report, _ := loop.Analyze(ctx)
fmt.Printf("Total: %d, High confidence: %d, Stale: %d\n",
    report.TotalMemories, report.HighConfidence, report.Stale)

for _, action := range report.RecommendedActions {
    fmt.Printf("  [P%d] %s: %s\n", action.Priority, action.Type, action.Description)
}
// Output:
//   [P3] prune: Remove 12 stale memories (>720h0m0s old)
//   [P2] validate: Re-validate 3 low-confidence memories (<0.3)
```

### Default Configuration

| Parameter            | Default    | Description                                   |
|----------------------|------------|-----------------------------------------------|
| Interval             | 1 hour     | How often the loop scans                      |
| StaleThreshold       | 30 days    | Memories older than this are flagged stale     |
| LowConfidenceLimit   | 0.3        | Entries below this confidence are flagged      |
| MaxMemoriesPerScan   | 500        | Maximum entries to analyze per scan            |

### Dependencies

Requires any MemoryProvider.

---

## 7. Memory Snapshots and Rollback

**Package:** `pkg/features/snapshots`

Creates point-in-time snapshots of the entire memory state for backup, comparison, and rollback. Snapshots deep-copy all memory entries and their metadata.

### Key Types

- `Manager` -- Manages snapshot lifecycle.
- `Snapshot` -- A captured memory state with ID, name, entries, and timestamp.
- `SnapshotDiff` -- Differences between two snapshots (added, removed entries).

### Key API Methods

```go
mgr := snapshots.NewManager(memoryProvider)

// Create a snapshot
snap, err := mgr.CreateSnapshot(ctx, "before-migration")

// List all snapshots
snaps := mgr.ListSnapshots()

// Compare two snapshots
diff, err := mgr.CompareSnapshots(snapID1, snapID2)
fmt.Printf("Added: %d, Removed: %d\n", len(diff.Added), len(diff.Removed))

// Delete a snapshot
mgr.DeleteSnapshot(snapID)
```

### Example

```go
mgr := snapshots.NewManager(unified)

// Take a snapshot before a risky operation
before, _ := mgr.CreateSnapshot(ctx, "pre-refactor")

// ... perform memory operations ...

// Take another snapshot
after, _ := mgr.CreateSnapshot(ctx, "post-refactor")

// See what changed
diff, _ := mgr.CompareSnapshots(before.ID, after.ID)
fmt.Printf("%d memories added, %d removed\n", len(diff.Added), len(diff.Removed))
```

### Dependencies

Requires any MemoryProvider with working `Search` (uses `*` query to fetch all entries).

---

## 8. Memory-Augmented AI Debate

**Package:** `pkg/features/debate_memory`

Provides memory-backed context injection for HelixAgent's AI debate system. Before a debate begins, the augmenter retrieves relevant memories, past decisions, and detected patterns to enrich the debate context.

### Key Types

- `Augmenter` -- The debate memory augmenter.
- `DebateContext` -- Retrieved context: relevant memories, past decisions, patterns.

### Key API Methods

```go
augmenter := debate_memory.NewAugmenter(memoryProvider)

// Get memory context for a debate
debateCtx, err := augmenter.GetDebateContext(ctx, "Should we use gRPC or REST?", sessionID)
// debateCtx.RelevantMemories, debateCtx.PastDecisions, debateCtx.Patterns

// Store debate outcome
augmenter.StoreDebateOutcome(ctx, sessionID, "gRPC vs REST", "Use gRPC for inter-service", 0.87)

// Store individual agent insight
augmenter.StoreAgentInsight(ctx, sessionID, "agent-critic", "REST has better tooling support")
```

### Example

```go
augmenter := debate_memory.NewAugmenter(unified)

debateCtx, _ := augmenter.GetDebateContext(ctx,
    "Best error handling strategy for Go microservices", "debate-42")

fmt.Printf("Retrieved %d memories in %v\n",
    debateCtx.TotalRetrieved, debateCtx.RetrievalTime)
fmt.Printf("  Relevant: %d, Past decisions: %d, Patterns: %d\n",
    len(debateCtx.RelevantMemories),
    len(debateCtx.PastDecisions),
    len(debateCtx.Patterns))

// After debate concludes, store the outcome
augmenter.StoreDebateOutcome(ctx, "debate-42",
    "Go error handling",
    "Use error wrapping with %w and sentinel errors",
    0.92)
```

### Context Retrieval Strategy

The augmenter performs three separate searches:
1. **Relevant memories**: General search by topic (top 20 results).
2. **Past decisions**: Search for "debate decision {topic}" filtered to `fact` type (top 10).
3. **Patterns**: Search for extracted keywords filtered to `procedural` type (top 10).

### Dependencies

Requires any MemoryProvider. Stores debate outcomes as `fact` type and agent insights as `episodic` type.

---

## 9. Adaptive Context Window Engineering

**Package:** `pkg/features/context_window`

Dynamically manages context windows by selecting the most relevant memories to fit within token limits. Uses priority-based greedy packing to maximize context quality within budget.

### Key Types

- `ContextWindow` -- The context window manager.
- `ContextBlock` -- A piece of context with content, source, priority, and token count.
- `ManagedContext` -- The output: a token-budgeted set of blocks with utilization metrics.

### Key API Methods

```go
cw := context_window.NewContextWindow(memoryProvider, 8192)
cw.SetReserved(2000) // Reserve tokens for system prompt + response

managed, err := cw.Build(ctx, "implement a REST API handler", "user-123")
fmt.Printf("Packed %d blocks, %d tokens (%.0f%% utilization), %d dropped\n",
    len(managed.Blocks), managed.TotalTokens,
    managed.Utilization*100, managed.Dropped)
```

### Example

```go
cw := context_window.NewContextWindow(unified, 4096)

managed, _ := cw.Build(ctx, "write a database migration", "user-123")

// Inject the blocks into the LLM prompt
for _, block := range managed.Blocks {
    fmt.Printf("[%s] (priority: %.2f, tokens: %d) %s\n",
        block.Source, block.Priority, block.TokenCount, block.Content[:80])
}
```

### Packing Algorithm

1. Fetch up to 50 candidate memories via search.
2. Convert each to a `ContextBlock` with priority: `relevance * 0.6 + confidence * 0.4`.
3. Sort by priority (highest first).
4. Greedily pack blocks until the token budget is exhausted.
5. Report utilization and number of dropped blocks.

Token estimation: average of word-based (`words * 4/3`) and character-based (`chars / 4`) estimates.

### Dependencies

Requires any MemoryProvider.

---

## 10. Cross-Project Knowledge Transfer

**Package:** `pkg/features/cross_project`

Transfers learned patterns, conventions, and domain knowledge between different projects. Useful when starting a new project that should follow patterns from an existing one.

### Key Types

- `Transferor` -- Manages cross-project knowledge transfer.
- `TransferableKnowledge` -- Knowledge grouped by category with confidence score.
- `TransferResult` -- Transfer report: counts of transferred, skipped, failed entries.

### Key API Methods

```go
transferor := cross_project.NewTransferor(memoryProvider)

// Identify what could be transferred
knowledge, err := transferor.IdentifyTransferable(ctx, "ProjectA", "ProjectB context")

// Execute the transfer
result, err := transferor.Transfer(ctx, "ProjectA", "ProjectB")
fmt.Printf("Transferred: %d, Failed: %d, Duration: %v\n",
    result.Transferred, result.Failed, result.Duration)
```

### Example

```go
transferor := cross_project.NewTransferor(unified)

// Transfer Go patterns from HelixAgent to a new project
result, _ := transferor.Transfer(ctx, "HelixAgent", "NewService")

fmt.Printf("Transferred %d knowledge entries from HelixAgent to NewService in %v\n",
    result.Transferred, result.Duration)
// Transferred entries have:
//   metadata.transferred_from = "HelixAgent"
//   metadata.transferred_to = "NewService"
//   confidence reduced by 10% (0.9 multiplier)
```

### Transfer Pipeline

1. Search for DNA entries from the source project (`[DNA:{project}]` query).
2. Group entries by category (pattern, convention, etc.).
3. For each entry, create a new memory for the target project with reduced confidence (90% of original).
4. Add transfer provenance metadata.

### Dependencies

Requires any MemoryProvider. Works best when Codebase DNA Profiling has been run on the source project.

---

## 11. MCP Bridge

**Package:** `pkg/features/mcp_bridge`

Exposes HelixMemory operations as MCP (Model Context Protocol) tools, enabling external AI tools and agents to interact with the memory system through the standard MCP interface.

### Key Types

- `Bridge` -- The MCP bridge exposing memory tools.
- `Tool` -- An MCP tool definition with name, description, and input schema.
- `ToolCall` -- An incoming tool call with name and JSON input.
- `ToolResult` -- The result of a tool call.

### Exposed MCP Tools

| Tool Name        | Description                                          |
|------------------|------------------------------------------------------|
| `memory_search`  | Search unified memory across all backends             |
| `memory_add`     | Add a new memory to the system                        |
| `memory_health`  | Check health of all memory backends                   |
| `memory_get`     | Retrieve a specific memory by ID                      |
| `memory_delete`  | Delete a memory by ID                                 |

### Key API Methods

```go
bridge := mcp_bridge.NewBridge(memoryProvider)

// List available tools (for MCP tool discovery)
tools := bridge.ListTools()

// Handle an incoming tool call
result := bridge.HandleToolCall(ctx, &mcp_bridge.ToolCall{
    Name:  "memory_search",
    Input: json.RawMessage(`{"query": "user preferences", "top_k": 5}`),
})
fmt.Println(result.Content)
```

### Example

```go
bridge := mcp_bridge.NewBridge(unified)

// Simulate an MCP tool call
call := &mcp_bridge.ToolCall{
    Name:  "memory_add",
    Input: json.RawMessage(`{
        "content": "User prefers dark mode",
        "type": "fact",
        "user_id": "user-123"
    }`),
}

result := bridge.HandleToolCall(ctx, call)
if result.IsError {
    fmt.Printf("Error: %s\n", result.Content)
} else {
    fmt.Println(result.Content) // "memory added successfully"
}
```

### Dependencies

Requires any MemoryProvider. No additional infrastructure needed.

---

## 12. Memory-Driven Code Generation

**Package:** `pkg/features/code_gen`

Leverages stored coding patterns, conventions, and project DNA to provide context-aware code generation assistance. Builds augmented prompts that include relevant memories.

### Key Types

- `Generator` -- The code generation context provider.
- `CodeContext` -- Retrieved context: patterns, conventions, examples, and an augmented prompt.

### Key API Methods

```go
gen := code_gen.NewGenerator(memoryProvider)

// Get code-relevant memory context
codeCtx, err := gen.GetCodeContext(ctx,
    "implement a REST handler with pagination",
    "go",
    "HelixAgent")

// Use the augmented prompt
fmt.Println(codeCtx.Prompt)
```

### Example

```go
gen := code_gen.NewGenerator(unified)

codeCtx, _ := gen.GetCodeContext(ctx,
    "write unit tests for the fusion engine",
    "go",
    "HelixMemory")

fmt.Printf("Found %d patterns, %d conventions, %d examples\n",
    len(codeCtx.Patterns),
    len(codeCtx.Conventions),
    len(codeCtx.Examples))

// The augmented prompt includes:
// - Task description
// - Known patterns from memory
// - Project conventions from memory
// - Related code examples from memory
fmt.Println(codeCtx.Prompt)
```

### Prompt Augmentation

The generator performs three searches and builds a structured prompt:

1. **Patterns**: Search for `"pattern {language} {project}"` filtered to `procedural` type.
2. **Conventions**: Search for `"convention style {language} {project}"`.
3. **Examples**: Search for the task description directly (top 5 results).

The results are formatted into a prompt with sections for Known Patterns, Conventions, and Related Examples, each truncated to prevent excessive length.

### Dependencies

Requires any MemoryProvider. Works best when Codebase DNA Profiling has been run on the target project.

---

## Feature Dependency Matrix

| Feature              | Requires Specific Backend? | Stores Memories? | Background Process? |
|----------------------|---------------------------|-------------------|---------------------|
| Codebase DNA         | No                        | Yes (procedural)  | No                  |
| Procedural Memory    | No                        | Yes (procedural)  | No                  |
| Multi-Agent Mesh     | No                        | Yes (any type)    | No                  |
| Temporal Reasoning   | Best with Graphiti        | No (reads only)   | No                  |
| Confidence Scoring   | No (self-contained)       | No                | No                  |
| Quality Loop         | No                        | No (reads + suggests) | Yes (periodic)  |
| Snapshots            | No                        | No (reads only)   | No                  |
| Debate Memory        | No                        | Yes (fact, episodic) | No               |
| Context Window       | No                        | No (reads only)   | No                  |
| Cross-Project        | No                        | Yes (transferred) | No                  |
| MCP Bridge           | No                        | Yes (via tools)   | No                  |
| Code Generation      | No                        | No (reads only)   | No                  |
