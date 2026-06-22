// Package wal is a durable, append-only Write-Ahead Log for the record-create
// path. A record is appended and fsync'd BEFORE it is inserted into MongoDB, so a
// crash between the two is recoverable: on startup the log is replayed (idempotently)
// to persist any record that did not make it to MongoDB.
//
// Design notes (fixing the defects of the previous WAL):
//   - Replay is idempotent: a record already in MongoDB surfaces as a duplicate,
//     which the caller treats as success, so an entry is never retained forever
//     (no "poison loop").
//   - All file operations are serialized under a single mutex, so an append can
//     never race a truncate (no lost-on-rewrite window).
//   - The log is truncated only when fully drained (no in-flight record), so a
//     concurrently appended entry is never discarded.
package wal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
)

// maxEntryBytes bounds a single scanned WAL line (records are size-capped at the
// HTTP edge, so this is generous headroom).
const maxEntryBytes = 16 * 1024 * 1024

// Stats describes the WAL state. PendingEntries counts ENTRIES (not bytes).
type Stats struct {
	Enabled        bool  `json:"enabled"`
	PendingEntries int   `json:"pending_entries"`
	TotalAppended  int64 `json:"total_appended"`
}

// RecordLog is the durability log the record-create path writes through.
type RecordLog interface {
	Append(rec *models.Record) error
	Confirm()
	Stats() Stats
}

// FileWAL is a local, append-only, fsync'd record log.
type FileWAL struct {
	path string

	mu       sync.Mutex
	file     *os.File
	pending  int   // appended but not yet confirmed/recovered (in-flight)
	count    int   // entries currently in the file
	appended int64 // lifetime appends (for stats)
}

// New opens (or creates) a file WAL under dir.
func New(dir string) (*FileWAL, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("wal: create dir: %w", err)
	}
	path := filepath.Join(dir, "records.wal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("wal: open: %w", err)
	}
	w := &FileWAL{path: path, file: f}
	// Account for entries left by a previous run (recovered by Recover).
	n, err := countEntries(path)
	if err != nil {
		f.Close()
		return nil, err
	}
	w.count = n
	w.pending = n
	return w, nil
}

func countEntries(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("wal: stat: %w", err)
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxEntryBytes)
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			n++
		}
	}
	return n, sc.Err()
}

// Append durably writes a record to the log (fsync) before returning.
func (w *FileWAL) Append(rec *models.Record) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("wal: marshal: %w", err)
	}
	line = append(line, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Write(line); err != nil {
		return fmt.Errorf("wal: write: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("wal: fsync: %w", err)
	}
	w.pending++
	w.count++
	w.appended++
	return nil
}

// Confirm marks one in-flight record as persisted downstream. When nothing is in
// flight, the log is truncated (everything appended is safely in MongoDB).
func (w *FileWAL) Confirm() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending > 0 {
		w.pending--
	}
	if w.pending == 0 {
		w.truncateLocked()
	}
}

// truncateLocked empties the log. The caller must hold w.mu and must have ensured
// nothing is in flight, so no concurrently appended entry can be lost.
func (w *FileWAL) truncateLocked() {
	if err := w.file.Truncate(0); err != nil {
		return
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return
	}
	w.count = 0
}

// Stats reports the WAL state (PendingEntries is a count of entries, not bytes).
func (w *FileWAL) Stats() Stats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Stats{Enabled: true, PendingEntries: w.count, TotalAppended: w.appended}
}

// Recover replays the log in append order, calling insert for each entry; insert
// must be idempotent (a record already persisted must be reported as success).
// On a real insert error the log is left intact for a later retry. On success the
// log is drained.
func (w *FileWAL) Recover(ctx context.Context, insert func(*models.Record) error) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("wal: seek: %w", err)
	}
	sc := bufio.NewScanner(w.file)
	sc.Buffer(make([]byte, 0, 64*1024), maxEntryBytes)

	recovered := 0
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return recovered, ctx.Err()
		default:
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r models.Record
		if err := json.Unmarshal(line, &r); err != nil {
			continue // skip a corrupt (e.g. partially written) trailing entry
		}
		if err := insert(&r); err != nil {
			return recovered, fmt.Errorf("wal: replay insert: %w", err)
		}
		recovered++
	}
	if err := sc.Err(); err != nil {
		return recovered, fmt.Errorf("wal: scan: %w", err)
	}

	w.truncateLocked()
	w.pending = 0
	return recovered, nil
}

// Close closes the underlying file.
func (w *FileWAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// NoopWAL is used when the WAL is disabled.
type NoopWAL struct{}

func (NoopWAL) Append(*models.Record) error { return nil }
func (NoopWAL) Confirm()                     {}
func (NoopWAL) Stats() Stats                 { return Stats{Enabled: false} }
