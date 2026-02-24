-- ============================================================================
-- HelixMemory SQL Schema Definitions
-- Module: digital.vasic.helixmemory
-- Version: 1.0.0
--
-- This schema defines the relational data model for HelixMemory's unified
-- cognitive memory engine. It backs the types defined in pkg/types/types.go
-- and supports the fusion engine, consolidation pipeline, core memory blocks,
-- and temporal event tracking.
--
-- Infrastructure: PostgreSQL 15+
-- Extensions required: pgcrypto (for gen_random_uuid), pg_trgm (for text search)
-- ============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

-- Memory type categories matching types.MemoryType constants.
CREATE TYPE memory_type AS ENUM (
    'fact',        -- Extracted facts (Mem0 primary)
    'graph',       -- Knowledge graph entries (Cognee primary)
    'core',        -- Persona/context memory (Letta primary)
    'temporal',    -- Time-aware memories (Graphiti primary)
    'episodic',    -- Conversation/event memories
    'procedural'   -- Learned workflows
);

-- Memory source identifiers matching types.MemorySource constants.
CREATE TYPE memory_source AS ENUM (
    'mem0',        -- Mem0 backend
    'cognee',      -- Cognee backend
    'letta',       -- Letta backend
    'graphiti',    -- Graphiti temporal layer
    'fusion'       -- Fused from multiple sources
);

-- Circuit breaker states matching types.CircuitState constants.
CREATE TYPE circuit_state AS ENUM (
    'closed',      -- Operating normally
    'open',        -- Tripped, calls rejected
    'half_open'    -- Testing recovery
);

-- ============================================================================
-- TABLE: memory_entries
-- Stores unified MemoryEntry records from all backends.
-- Maps to types.MemoryEntry struct.
-- ============================================================================

CREATE TABLE memory_entries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content         TEXT NOT NULL,
    type            memory_type NOT NULL DEFAULT 'fact',
    source          memory_source NOT NULL DEFAULT 'fusion',
    confidence      DOUBLE PRECISION NOT NULL DEFAULT 0.0
                        CHECK (confidence >= 0.0 AND confidence <= 1.0),
    relevance       DOUBLE PRECISION NOT NULL DEFAULT 0.0
                        CHECK (relevance >= 0.0 AND relevance <= 1.0),
    metadata        JSONB DEFAULT '{}'::jsonb,
    embedding       REAL[],  -- float32 vector for similarity search
    user_id         VARCHAR(255),
    session_id      VARCHAR(255),
    agent_id        VARCHAR(255),
    tags            TEXT[] DEFAULT '{}',
    access_count    INTEGER NOT NULL DEFAULT 0,
    last_access     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for memory_entries
CREATE INDEX idx_memory_entries_type ON memory_entries (type);
CREATE INDEX idx_memory_entries_source ON memory_entries (source);
CREATE INDEX idx_memory_entries_user_id ON memory_entries (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_memory_entries_session_id ON memory_entries (session_id) WHERE session_id IS NOT NULL;
CREATE INDEX idx_memory_entries_agent_id ON memory_entries (agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX idx_memory_entries_confidence ON memory_entries (confidence DESC);
CREATE INDEX idx_memory_entries_relevance ON memory_entries (relevance DESC);
CREATE INDEX idx_memory_entries_created_at ON memory_entries (created_at DESC);
CREATE INDEX idx_memory_entries_updated_at ON memory_entries (updated_at DESC);
CREATE INDEX idx_memory_entries_expires_at ON memory_entries (expires_at)
    WHERE expires_at IS NOT NULL;
CREATE INDEX idx_memory_entries_tags ON memory_entries USING GIN (tags);
CREATE INDEX idx_memory_entries_metadata ON memory_entries USING GIN (metadata jsonb_path_ops);
CREATE INDEX idx_memory_entries_content_trgm ON memory_entries
    USING GIN (content gin_trgm_ops);

-- Composite indexes for common query patterns
CREATE INDEX idx_memory_entries_user_type ON memory_entries (user_id, type)
    WHERE user_id IS NOT NULL;
CREATE INDEX idx_memory_entries_source_created ON memory_entries (source, created_at DESC);

-- ============================================================================
-- TABLE: memory_sources
-- Tracks registered backend sources and their health/configuration state.
-- ============================================================================

CREATE TABLE memory_sources (
    source          memory_source PRIMARY KEY,
    endpoint        VARCHAR(1024) NOT NULL,
    display_name    VARCHAR(255) NOT NULL,
    description     TEXT,
    is_healthy      BOOLEAN NOT NULL DEFAULT FALSE,
    circuit_state   circuit_state NOT NULL DEFAULT 'closed',
    failure_count   INTEGER NOT NULL DEFAULT 0,
    last_health_check TIMESTAMPTZ,
    last_failure    TIMESTAMPTZ,
    config          JSONB DEFAULT '{}'::jsonb,
    trust_score     DOUBLE PRECISION NOT NULL DEFAULT 0.5
                        CHECK (trust_score >= 0.0 AND trust_score <= 1.0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default backend sources
INSERT INTO memory_sources (source, endpoint, display_name, description, trust_score) VALUES
    ('mem0',     'http://localhost:8001', 'Mem0',     'Dynamic fact extraction and preference management', 0.85),
    ('cognee',   'http://localhost:8000', 'Cognee',   'Semantic knowledge graphs via ECL pipeline',        0.80),
    ('letta',    'http://localhost:8283', 'Letta',    'Stateful agent runtime with core memory blocks',    0.95),
    ('graphiti', 'http://localhost:8003', 'Graphiti', 'Temporal knowledge graph with bi-temporal queries', 0.85),
    ('fusion',   'internal',             'Fusion',   'Fused results from multiple sources',               0.90)
ON CONFLICT (source) DO NOTHING;

-- ============================================================================
-- TABLE: fusion_scores
-- Stores fusion scoring results from the 3-stage fusion engine.
-- Maps to the output of fusion.Engine.Fuse().
-- ============================================================================

CREATE TABLE fusion_scores (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_entry_id     UUID NOT NULL REFERENCES memory_entries(id) ON DELETE CASCADE,
    query_hash          VARCHAR(64) NOT NULL,   -- SHA-256 of the search query
    relevance_score     DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    recency_score       DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    source_score        DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    type_score          DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    final_score         DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    is_deduplicated     BOOLEAN NOT NULL DEFAULT FALSE,
    dedup_group_id      UUID,           -- Groups duplicate entries together
    dedup_similarity    DOUBLE PRECISION,  -- Cosine/Jaccard sim that triggered dedup
    sources_queried     memory_source[] NOT NULL DEFAULT '{}',
    fusion_duration_ms  INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for fusion_scores
CREATE INDEX idx_fusion_scores_entry_id ON fusion_scores (memory_entry_id);
CREATE INDEX idx_fusion_scores_query_hash ON fusion_scores (query_hash);
CREATE INDEX idx_fusion_scores_final_score ON fusion_scores (final_score DESC);
CREATE INDEX idx_fusion_scores_created_at ON fusion_scores (created_at DESC);
CREATE INDEX idx_fusion_scores_dedup_group ON fusion_scores (dedup_group_id)
    WHERE dedup_group_id IS NOT NULL;

-- Composite index for query result lookups
CREATE INDEX idx_fusion_scores_query_score ON fusion_scores (query_hash, final_score DESC);

-- ============================================================================
-- TABLE: consolidation_log
-- Tracks sleep-time compute runs from the consolidation engine.
-- Maps to consolidation.Stats and types.ConsolidationStatus.
-- ============================================================================

CREATE TABLE consolidation_log (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_number          SERIAL,
    status              VARCHAR(20) NOT NULL DEFAULT 'running'
                            CHECK (status IN ('running', 'completed', 'failed')),
    memories_processed  INTEGER NOT NULL DEFAULT 0,
    deduplicated        INTEGER NOT NULL DEFAULT 0,
    consolidated        INTEGER NOT NULL DEFAULT 0,
    errors              INTEGER NOT NULL DEFAULT 0,
    error_details       TEXT[],
    batch_size          INTEGER NOT NULL DEFAULT 100,
    providers_queried   memory_source[] NOT NULL DEFAULT '{}',
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    metadata            JSONB DEFAULT '{}'::jsonb
);

-- Indexes for consolidation_log
CREATE INDEX idx_consolidation_log_status ON consolidation_log (status);
CREATE INDEX idx_consolidation_log_started_at ON consolidation_log (started_at DESC);
CREATE INDEX idx_consolidation_log_run_number ON consolidation_log (run_number DESC);

-- ============================================================================
-- TABLE: core_memory_blocks
-- Stores Letta-style editable in-context memory blocks.
-- Maps to types.CoreMemoryBlock struct.
-- ============================================================================

CREATE TABLE core_memory_blocks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        VARCHAR(255) NOT NULL,
    label           VARCHAR(255) NOT NULL,
    value           TEXT NOT NULL DEFAULT '',
    char_limit      INTEGER NOT NULL DEFAULT 5000
                        CHECK (char_limit > 0),
    version         INTEGER NOT NULL DEFAULT 1,
    previous_value  TEXT,           -- Value before last update (for rollback)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_core_memory_agent_label UNIQUE (agent_id, label)
);

-- Indexes for core_memory_blocks
CREATE INDEX idx_core_memory_blocks_agent_id ON core_memory_blocks (agent_id);
CREATE INDEX idx_core_memory_blocks_label ON core_memory_blocks (label);
CREATE INDEX idx_core_memory_blocks_updated_at ON core_memory_blocks (updated_at DESC);

-- Seed default blocks for the helixmemory agent
INSERT INTO core_memory_blocks (agent_id, label, value, char_limit) VALUES
    ('helixmemory', 'human',           'The user interacting with HelixAgent.', 5000),
    ('helixmemory', 'persona',         'I am the HelixMemory system, managing unified cognitive memory across Mem0, Cognee, and Letta backends.', 5000),
    ('helixmemory', 'project_context', '', 10000),
    ('helixmemory', 'working_memory',  '', 10000)
ON CONFLICT (agent_id, label) DO NOTHING;

-- ============================================================================
-- TABLE: temporal_events
-- Stores Graphiti temporal events with bi-temporal validity tracking.
-- Supports "what was true at time T?" queries from the temporal Reasoner.
-- ============================================================================

CREATE TABLE temporal_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_entry_id UUID NOT NULL REFERENCES memory_entries(id) ON DELETE CASCADE,
    valid_at        TIMESTAMPTZ NOT NULL,       -- When the fact became true
    invalid_at      TIMESTAMPTZ,                -- When the fact became false (NULL = still valid)
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    event_type      VARCHAR(100),               -- Category of temporal event
    subject         VARCHAR(500),               -- Entity the event is about
    predicate       VARCHAR(500),               -- Relationship or property
    object          VARCHAR(500),               -- Target entity or value
    source_ref      VARCHAR(1024),              -- Original source reference
    metadata        JSONB DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for temporal_events
CREATE INDEX idx_temporal_events_entry_id ON temporal_events (memory_entry_id);
CREATE INDEX idx_temporal_events_valid_at ON temporal_events (valid_at);
CREATE INDEX idx_temporal_events_invalid_at ON temporal_events (invalid_at)
    WHERE invalid_at IS NOT NULL;
CREATE INDEX idx_temporal_events_is_active ON temporal_events (is_active)
    WHERE is_active = TRUE;
CREATE INDEX idx_temporal_events_subject ON temporal_events (subject)
    WHERE subject IS NOT NULL;
CREATE INDEX idx_temporal_events_event_type ON temporal_events (event_type)
    WHERE event_type IS NOT NULL;

-- Composite index for bi-temporal range queries
-- Supports: "What was true at time T?" and "What changed between T1 and T2?"
CREATE INDEX idx_temporal_events_validity ON temporal_events (valid_at, invalid_at)
    WHERE is_active = TRUE;
CREATE INDEX idx_temporal_events_subject_time ON temporal_events (subject, valid_at DESC)
    WHERE subject IS NOT NULL;

-- ============================================================================
-- TABLE: memory_snapshots
-- Stores point-in-time snapshot metadata for the snapshots feature.
-- Maps to snapshots.Snapshot struct.
-- ============================================================================

CREATE TABLE memory_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    entry_count     INTEGER NOT NULL DEFAULT 0,
    metadata        JSONB DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_memory_snapshots_name ON memory_snapshots (name);
CREATE INDEX idx_memory_snapshots_created_at ON memory_snapshots (created_at DESC);

-- Snapshot entry references (which entries were in the snapshot)
CREATE TABLE memory_snapshot_entries (
    snapshot_id     UUID NOT NULL REFERENCES memory_snapshots(id) ON DELETE CASCADE,
    entry_id        UUID NOT NULL REFERENCES memory_entries(id) ON DELETE CASCADE,
    entry_snapshot  JSONB NOT NULL,  -- Deep copy of entry state at snapshot time
    PRIMARY KEY (snapshot_id, entry_id)
);

CREATE INDEX idx_snapshot_entries_snapshot_id ON memory_snapshot_entries (snapshot_id);

-- ============================================================================
-- FUNCTIONS AND TRIGGERS
-- ============================================================================

-- Auto-update updated_at timestamp on row modification.
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_memory_entries_updated_at
    BEFORE UPDATE ON memory_entries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_memory_sources_updated_at
    BEFORE UPDATE ON memory_sources
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_core_memory_blocks_updated_at
    BEFORE UPDATE ON core_memory_blocks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_temporal_events_updated_at
    BEFORE UPDATE ON temporal_events
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Auto-increment access_count on retrieval (called explicitly).
CREATE OR REPLACE FUNCTION record_memory_access(p_id UUID)
RETURNS VOID AS $$
BEGIN
    UPDATE memory_entries
    SET access_count = access_count + 1,
        last_access = NOW()
    WHERE id = p_id;
END;
$$ LANGUAGE plpgsql;

-- Auto-increment core memory block version on update.
CREATE OR REPLACE FUNCTION increment_block_version()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.value IS DISTINCT FROM NEW.value THEN
        NEW.version = OLD.version + 1;
        NEW.previous_value = OLD.value;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_core_memory_blocks_version
    BEFORE UPDATE ON core_memory_blocks
    FOR EACH ROW EXECUTE FUNCTION increment_block_version();

-- Clean up expired memory entries (run periodically).
CREATE OR REPLACE FUNCTION cleanup_expired_memories()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM memory_entries
    WHERE expires_at IS NOT NULL AND expires_at < NOW();
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- VIEWS
-- ============================================================================

-- Active memory entries (not expired).
CREATE OR REPLACE VIEW v_active_memories AS
SELECT *
FROM memory_entries
WHERE expires_at IS NULL OR expires_at > NOW();

-- Memory entry counts by type and source.
CREATE OR REPLACE VIEW v_memory_stats AS
SELECT
    type,
    source,
    COUNT(*) AS entry_count,
    AVG(confidence) AS avg_confidence,
    AVG(relevance) AS avg_relevance,
    AVG(access_count) AS avg_access_count,
    MIN(created_at) AS oldest,
    MAX(created_at) AS newest
FROM memory_entries
GROUP BY type, source;

-- Currently active temporal events.
CREATE OR REPLACE VIEW v_active_temporal_events AS
SELECT
    te.*,
    me.content,
    me.confidence
FROM temporal_events te
JOIN memory_entries me ON te.memory_entry_id = me.id
WHERE te.is_active = TRUE
  AND (te.invalid_at IS NULL OR te.invalid_at > NOW());

-- Latest consolidation run summary.
CREATE OR REPLACE VIEW v_latest_consolidation AS
SELECT *
FROM consolidation_log
ORDER BY started_at DESC
LIMIT 1;

-- Source health overview.
CREATE OR REPLACE VIEW v_source_health AS
SELECT
    ms.source,
    ms.display_name,
    ms.endpoint,
    ms.is_healthy,
    ms.circuit_state,
    ms.failure_count,
    ms.trust_score,
    ms.last_health_check,
    COALESCE(counts.entry_count, 0) AS entry_count
FROM memory_sources ms
LEFT JOIN (
    SELECT source, COUNT(*) AS entry_count
    FROM memory_entries
    GROUP BY source
) counts ON ms.source = counts.source;
