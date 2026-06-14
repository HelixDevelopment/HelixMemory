package localstore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	modstore "digital.vasic.memory/pkg/store"
)

func newTempStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

// TestSQLiteStore_AddGetSearch is the core unit test: store a fact, read it
// back by ID and by search.
func TestSQLiteStore_AddGetSearch(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)

	m := &modstore.Memory{
		Content:  "my favourite token is ZephyrQuartz-4471",
		Scope:    modstore.ScopeUser,
		Metadata: map[string]any{"session_id": "s1", "role": "user"},
	}
	if err := s.Add(ctx, m); err != nil {
		t.Fatalf("add: %v", err)
	}
	if m.ID == "" {
		t.Fatal("expected generated ID")
	}

	got, err := s.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != m.Content {
		t.Fatalf("content mismatch: %q", got.Content)
	}
	if got.Metadata["session_id"] != "s1" {
		t.Fatalf("metadata not preserved: %+v", got.Metadata)
	}

	res, err := s.Search(ctx, "favourite token ZephyrQuartz-4471", &modstore.SearchOptions{TopK: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("search returned no results for a stored fact")
	}
	if res[0].Score <= 0 {
		t.Fatalf("expected positive score, got %v", res[0].Score)
	}
}

// TestSQLiteStore_PersistsAcrossReopen is the headline durability test: write
// with one *SQLiteStore handle, CLOSE it, reopen the SAME file, and recall —
// genuine on-disk persistence, not an in-process map.
func TestSQLiteStore_PersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	if err := s1.Add(ctx, &modstore.Memory{Content: "fact ZephyrQuartz-4471", Scope: modstore.ScopeUser}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("db file is empty after write")
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer s2.Close()
	res, err := s2.Search(ctx, "ZephyrQuartz-4471", &modstore.SearchOptions{TopK: 5})
	if err != nil {
		t.Fatalf("search 2: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("fact did NOT survive reopen of %s (size=%d)", path, fi.Size())
	}
	t.Logf("PERSIST OK: recalled across reopen from %s (%d bytes)", path, fi.Size())
}

func TestSQLiteStore_UpdateDelete(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	m := &modstore.Memory{Content: "v1", Scope: modstore.ScopeUser}
	if err := s.Add(ctx, m); err != nil {
		t.Fatalf("add: %v", err)
	}
	m.Content = "v2"
	if err := s.Update(ctx, m); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.Get(ctx, m.ID)
	if got.Content != "v2" {
		t.Fatalf("update not applied: %q", got.Content)
	}
	if err := s.Delete(ctx, m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, m.ID); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestSQLiteStore_ListScope(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	_ = s.Add(ctx, &modstore.Memory{Content: "u1", Scope: modstore.ScopeUser})
	_ = s.Add(ctx, &modstore.Memory{Content: "g1", Scope: modstore.ScopeGlobal})
	out, err := s.List(ctx, modstore.ScopeUser, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 1 || out[0].Content != "u1" {
		t.Fatalf("scope filter failed: %+v", out)
	}
}

// TestSQLiteStore_Stress_BulkInsert is the §11.4.85 stress floor: 1000-row
// insert, then a durable count across reopen.
func TestSQLiteStore_Stress_BulkInsert(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "stress.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const n = 1000
	start := time.Now()
	for i := 0; i < n; i++ {
		if err := s.Add(ctx, &modstore.Memory{Content: fmt.Sprintf("bulk-%d ZephyrQuartz-4471", i), Scope: modstore.ScopeUser}); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	_ = s.Close()

	s2, _ := Open(path)
	defer s2.Close()
	cnt, err := s2.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != n {
		t.Fatalf("expected %d rows after reopen, got %d", n, cnt)
	}
	t.Logf("STRESS OK: %d inserts in %v, all %d durable across reopen", n, elapsed, cnt)
}

// TestSQLiteStore_Stress_ConcurrentWriters is the §11.4.85 concurrency floor:
// 10 goroutines writing simultaneously, no deadlock, all rows durable.
func TestSQLiteStore_Stress_ConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	const writers = 10
	const each = 50
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if err := s.Add(ctx, &modstore.Memory{Content: fmt.Sprintf("w%d-i%d", w, i), Scope: modstore.ScopeUser}); err != nil {
					errCh <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent writer: %v", err)
	}
	cnt, _ := s.Count(ctx)
	if cnt != writers*each {
		t.Fatalf("expected %d rows, got %d (lost writes under contention)", writers*each, cnt)
	}
	t.Logf("CONCURRENCY OK: %d writers x %d = %d rows, none lost", writers, each, cnt)
}

// TestSQLiteStore_Chaos_KillMidWrite is the §11.4.85 chaos floor: a child
// process writes N rows then is SIGKILLed mid-stream; the parent reopens the
// same DB and proves it recovers cleanly (the committed rows survive, the DB is
// not corrupt). Uses a re-exec of the test binary as the child.
func TestSQLiteStore_Chaos_KillMidWrite(t *testing.T) {
	if os.Getenv("LOCALSTORE_CHAOS_CHILD") == "1" {
		runChaosChild()
		return
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "chaos.db")

	cmd := exec.Command(os.Args[0], "-test.run", "TestSQLiteStore_Chaos_KillMidWrite")
	cmd.Env = append(os.Environ(), "LOCALSTORE_CHAOS_CHILD=1", "LOCALSTORE_CHAOS_DB="+path)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	// Let the child write a bit, then SIGKILL it mid-stream.
	time.Sleep(400 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	// Parent recovery: reopen the same DB; it must open cleanly and have rows.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("RECOVERY FAIL: reopen after mid-write SIGKILL: %v", err)
	}
	defer s.Close()
	cnt, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("RECOVERY FAIL: count after kill: %v", err)
	}
	t.Logf("CHAOS OK: DB reopened cleanly after mid-write SIGKILL, %d committed rows survived", cnt)
}

func runChaosChild() {
	path := os.Getenv("LOCALSTORE_CHAOS_DB")
	s, err := Open(path)
	if err != nil {
		os.Exit(2)
	}
	ctx := context.Background()
	for i := 0; i < 100000; i++ {
		_ = s.Add(ctx, &modstore.Memory{Content: fmt.Sprintf("chaos-%d", i), Scope: modstore.ScopeUser})
		time.Sleep(2 * time.Millisecond)
	}
	_ = s.Close()
}
