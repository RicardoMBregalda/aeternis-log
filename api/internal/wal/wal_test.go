package wal

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
)

func rec(id string) *models.Record {
	return &models.Record{Tenant: "t", Domain: "d", ID: id, Source: "s",
		Payload: map[string]interface{}{"id": id}, Hash: "h-" + id}
}

// collect returns an insert func that records every record it receives.
func collect(got *[]*models.Record) func(*models.Record) error {
	var mu sync.Mutex
	return func(r *models.Record) error {
		mu.Lock()
		*got = append(*got, r)
		mu.Unlock()
		return nil
	}
}

// TestAppendDurableAndRecovers (F10): records appended but not confirmed (the
// crash window) are replayed on recovery with their content intact.
func TestAppendDurableAndRecovers(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if err := w.Append(rec(id)); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}
	w.Close()

	// Simulate a restart: a fresh WAL on the same directory recovers the entries.
	w2, err := New(dir)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	var got []*models.Record
	n, err := w2.Recover(context.Background(), collect(&got))
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if n != 3 || len(got) != 3 {
		t.Fatalf("recovered %d (got %d), want 3", n, len(got))
	}
	for i, id := range []string{"a", "b", "c"} {
		if got[i].ID != id {
			t.Errorf("recovered[%d].ID = %s, want %s", i, got[i].ID, id)
		}
	}
	// After recovery the log is drained: a second recovery yields nothing.
	var again []*models.Record
	if n2, _ := w2.Recover(context.Background(), collect(&again)); n2 != 0 {
		t.Errorf("second recovery returned %d, want 0 (log must be drained)", n2)
	}
}

// TestRecoverIsIdempotent (F11): when replay hits records already persisted
// (insert reports them, here as success), the log still drains — it must not
// retain entries forever ("poison loop").
func TestRecoverIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	w, _ := New(dir)
	w.Append(rec("a"))
	w.Append(rec("b"))
	w.Close()

	w2, _ := New(dir)
	// insert always "succeeds" (as a no-op would for an already-present record).
	calls := 0
	idempotent := func(*models.Record) error { calls++; return nil }
	if _, err := w2.Recover(context.Background(), idempotent); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if calls != 2 {
		t.Fatalf("insert called %d times, want 2", calls)
	}
	if got := w2.Stats().PendingEntries; got != 0 {
		t.Errorf("after recovery PendingEntries = %d, want 0 (no retained entries)", got)
	}
}

// TestConfirmTruncatesWhenDrained: a confirmed record (persisted downstream)
// leaves nothing to recover.
func TestConfirmTruncatesWhenDrained(t *testing.T) {
	dir := t.TempDir()
	w, _ := New(dir)
	w.Append(rec("a"))
	w.Confirm()
	if got := w.Stats().PendingEntries; got != 0 {
		t.Errorf("PendingEntries = %d after confirm, want 0", got)
	}
	w.Close()

	w2, _ := New(dir)
	var got []*models.Record
	if n, _ := w2.Recover(context.Background(), collect(&got)); n != 0 {
		t.Errorf("recovered %d after confirm, want 0", n)
	}
}

// TestStatsCountsEntriesNotBytes (F13): PendingEntries is a count of entries, not
// the file size in bytes.
func TestStatsCountsEntriesNotBytes(t *testing.T) {
	dir := t.TempDir()
	w, _ := New(dir)
	for i := 0; i < 5; i++ {
		w.Append(rec(fmt.Sprintf("record-with-a-longish-id-%d", i)))
	}
	if got := w.Stats().PendingEntries; got != 5 {
		t.Errorf("PendingEntries = %d, want 5 (entries, not bytes)", got)
	}
}

// TestConcurrentAppendsNoLoss (F12): concurrent appends and a recovery never lose
// an entry. Run with -race.
func TestConcurrentAppendsNoLoss(t *testing.T) {
	dir := t.TempDir()
	w, _ := New(dir)

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := w.Append(rec(fmt.Sprintf("c-%03d", i))); err != nil {
				t.Errorf("Append: %v", err)
			}
		}(i)
	}
	wg.Wait()
	w.Close()

	w2, _ := New(dir)
	var got []*models.Record
	recovered, err := w2.Recover(context.Background(), collect(&got))
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if recovered != n {
		t.Errorf("recovered %d of %d appended entries (lost some)", recovered, n)
	}
}
