// Package localstore provides a zero-config, durable, embedded SQLite
// implementation of the digital.vasic.memory MemoryStore interface.
//
// It exists so HelixMemory can persist memories OUT-OF-THE-BOX with no external
// service and no API keys: the fusion engine's Cognee/Mem0/Letta/Graphiti
// clients are all nil without HELIX_MEMORY_* configuration, and the fusion
// engine has no local fallback — so absent external infra a Store call hard
// fails. This local store closes that gap with a pure-Go (cgo-free)
// modernc.org/sqlite file backend that satisfies the same MemoryStore contract
// the unified provider already adapts to.
//
// CONST-051(B) decoupling: this package is project-not-aware. It takes a
// filesystem path (constructor parameter) and nothing else — no HelixCode
// path, hostname, or asset is hardcoded. Any consuming project supplies the DB
// path; the default is derived from the OS user-config dir, overridable.
package localstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	modstore "digital.vasic.memory/pkg/store"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo)
)

// SQLiteStore is a durable embedded MemoryStore backed by a single SQLite file.
// It is safe for concurrent use. The underlying *sql.DB pools connections; an
// additional mutex serialises writes to keep SQLite's single-writer model happy
// without surfacing SQLITE_BUSY to callers under contention.
type SQLiteStore struct {
	db   *sql.DB
	path string
	wmu  sync.Mutex // serialise writers
}

// DefaultDBPath returns the zero-config default database path under the OS
// user-config directory: <user-config>/helix_memory/memory.db. It honours the
// HELIX_MEMORY_DB environment variable as an explicit override. The parent
// directory is created if missing.
func DefaultDBPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("HELIX_MEMORY_DB")); override != "" {
		return override, nil
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(cfgDir) == "" {
		// Fall back to the home dir, then the working dir — never fail closed
		// just because the config dir is unavailable.
		if home, herr := os.UserHomeDir(); herr == nil && home != "" {
			cfgDir = filepath.Join(home, ".config")
		} else {
			cfgDir = "."
		}
	}
	return filepath.Join(cfgDir, "helix_memory", "memory.db"), nil
}

// Open creates (or opens) a SQLiteStore at the given path. The parent directory
// is created with 0700 so the durable memory is not world-readable. Passing an
// empty path uses DefaultDBPath. Passing ":memory:" yields a private in-memory
// database (useful for tests).
func Open(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		def, err := DefaultDBPath()
		if err != nil {
			return nil, fmt.Errorf("localstore: resolve default db path: %w", err)
		}
		path = def
	}

	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("localstore: create db dir %q: %w", dir, err)
		}
	}

	dsn := path
	if path != ":memory:" {
		// WAL for durability+concurrency; busy_timeout so concurrent writers
		// wait rather than erroring; foreign_keys on for hygiene.
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("localstore: open sqlite %q: %w", path, err)
	}
	db.SetMaxOpenConns(1) // single connection keeps the WAL writer model simple + race-free

	s := &SQLiteStore{db: db, path: path}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS memories (
	id          TEXT PRIMARY KEY,
	content     TEXT NOT NULL,
	scope       TEXT NOT NULL DEFAULT 'user',
	metadata    TEXT NOT NULL DEFAULT '{}',
	embedding   BLOB,
	score       REAL NOT NULL DEFAULT 0,
	created_at  TIMESTAMP NOT NULL,
	updated_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("localstore: migrate: %w", err)
	}
	return nil
}

// Path returns the on-disk database path (or ":memory:").
func (s *SQLiteStore) Path() string { return s.path }

// Close releases the database. A WAL checkpoint is issued first so the -wal/-shm
// sidecars are folded back into the main file and are safely discardable.
func (s *SQLiteStore) Close() error {
	if s.path != ":memory:" {
		// Best-effort checkpoint; ignore error (DB may already be closing).
		_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	}
	return s.db.Close()
}

// Add stores a new memory. If the memory has no ID, one is generated.
func (s *SQLiteStore) Add(ctx context.Context, memory *modstore.Memory) error {
	if memory == nil {
		return fmt.Errorf("localstore: Add nil memory")
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()

	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = now
	}
	if memory.Scope == "" {
		memory.Scope = modstore.ScopeUser
	}

	metaJSON, err := json.Marshal(memory.Metadata)
	if err != nil {
		return fmt.Errorf("localstore: marshal metadata: %w", err)
	}
	embBlob, err := marshalEmbedding(memory.Embedding)
	if err != nil {
		return fmt.Errorf("localstore: marshal embedding: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO memories (id, content, scope, metadata, embedding, score, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   content=excluded.content, scope=excluded.scope, metadata=excluded.metadata,
		   embedding=excluded.embedding, score=excluded.score, updated_at=excluded.updated_at`,
		memory.ID, memory.Content, string(memory.Scope), string(metaJSON), embBlob,
		memory.Score, memory.CreatedAt, memory.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("localstore: insert memory: %w", err)
	}
	return nil
}

// Get retrieves a memory by ID.
func (s *SQLiteStore) Get(ctx context.Context, id string) (*modstore.Memory, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, content, scope, metadata, embedding, score, created_at, updated_at
		 FROM memories WHERE id = ?`, id)
	m, err := scanMemory(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("localstore: memory not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Update modifies an existing memory.
func (s *SQLiteStore) Update(ctx context.Context, memory *modstore.Memory) error {
	if memory == nil {
		return fmt.Errorf("localstore: Update nil memory")
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()

	memory.UpdatedAt = time.Now().UTC()
	metaJSON, err := json.Marshal(memory.Metadata)
	if err != nil {
		return fmt.Errorf("localstore: marshal metadata: %w", err)
	}
	embBlob, err := marshalEmbedding(memory.Embedding)
	if err != nil {
		return fmt.Errorf("localstore: marshal embedding: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET content=?, scope=?, metadata=?, embedding=?, score=?, updated_at=?
		 WHERE id=?`,
		memory.Content, string(memory.Scope), string(metaJSON), embBlob, memory.Score,
		memory.UpdatedAt, memory.ID,
	)
	if err != nil {
		return fmt.Errorf("localstore: update memory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("localstore: memory not found: %s", memory.ID)
	}
	return nil
}

// Delete removes a memory by ID.
func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()

	res, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("localstore: delete memory: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("localstore: memory not found: %s", id)
	}
	return nil
}

// Search returns memories matching the query string and options. It uses a
// case-insensitive word-overlap score over content (the same shape the unified
// provider's in-memory store uses) so results are deterministic and durable.
func (s *SQLiteStore) Search(ctx context.Context, query string, opts *modstore.SearchOptions) ([]*modstore.Memory, error) {
	if opts == nil {
		opts = modstore.DefaultSearchOptions()
	}

	where := []string{"1=1"}
	args := []any{}
	if opts.Scope != "" {
		where = append(where, "scope = ?")
		args = append(args, string(opts.Scope))
	}
	if opts.TimeRange != nil {
		where = append(where, "created_at >= ? AND created_at <= ?")
		args = append(args, opts.TimeRange.Start, opts.TimeRange.End)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content, scope, metadata, embedding, score, created_at, updated_at
		 FROM memories WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, fmt.Errorf("localstore: search query: %w", err)
	}
	defer rows.Close()

	queryWords := strings.Fields(strings.ToLower(query))
	var results []*modstore.Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		sc := matchScore(queryWords, m.Content)
		if sc >= opts.MinScore {
			m.Score = sc
			results = append(results, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("localstore: search rows: %w", err)
	}

	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if opts.TopK > 0 && len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results, nil
}

// List returns memories matching the scope and options.
func (s *SQLiteStore) List(ctx context.Context, scope modstore.Scope, opts *modstore.ListOptions) ([]*modstore.Memory, error) {
	if opts == nil {
		opts = modstore.DefaultListOptions()
	}

	where := []string{"1=1"}
	args := []any{}
	if scope != "" {
		where = append(where, "scope = ?")
		args = append(args, string(scope))
	} else if opts.Scope != "" {
		where = append(where, "scope = ?")
		args = append(args, string(opts.Scope))
	}

	order := "created_at ASC"
	switch opts.OrderBy {
	case "updated_at":
		order = "updated_at ASC"
	case "score":
		order = "score DESC"
	}

	q := `SELECT id, content, scope, metadata, embedding, score, created_at, updated_at
		  FROM memories WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + order
	if opts.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", opts.Limit, opts.Offset)
	} else if opts.Offset > 0 {
		q += fmt.Sprintf(" LIMIT -1 OFFSET %d", opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("localstore: list query: %w", err)
	}
	defer rows.Close()

	var results []*modstore.Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("localstore: list rows: %w", err)
	}
	return results, nil
}

// Count returns the total number of stored memories (helper, not part of the
// MemoryStore interface).
func (s *SQLiteStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&n)
	return n, err
}

// scanner abstracts *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanMemory(row scanner) (*modstore.Memory, error) {
	var (
		m        modstore.Memory
		scope    string
		metaJSON string
		embBlob  []byte
	)
	if err := row.Scan(&m.ID, &m.Content, &scope, &metaJSON, &embBlob, &m.Score, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	m.Scope = modstore.Scope(scope)
	if metaJSON != "" {
		if err := json.Unmarshal([]byte(metaJSON), &m.Metadata); err != nil {
			return nil, fmt.Errorf("localstore: unmarshal metadata: %w", err)
		}
	}
	emb, err := unmarshalEmbedding(embBlob)
	if err != nil {
		return nil, fmt.Errorf("localstore: unmarshal embedding: %w", err)
	}
	m.Embedding = emb
	return &m, nil
}

func marshalEmbedding(emb []float32) ([]byte, error) {
	if len(emb) == 0 {
		return nil, nil
	}
	return json.Marshal(emb)
}

func unmarshalEmbedding(b []byte) ([]float32, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var emb []float32
	if err := json.Unmarshal(b, &emb); err != nil {
		return nil, err
	}
	return emb, nil
}

func matchScore(queryWords []string, content string) float64 {
	if len(queryWords) == 0 {
		return 0
	}
	contentLower := strings.ToLower(content)
	matches := 0
	for _, w := range queryWords {
		if strings.Contains(contentLower, w) {
			matches++
		}
	}
	return float64(matches) / float64(len(queryWords))
}

// Ensure SQLiteStore satisfies the MemoryStore contract at compile time.
var _ modstore.MemoryStore = (*SQLiteStore)(nil)
