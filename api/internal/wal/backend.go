package wal

import "github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"

// WAL is the durability-log abstraction shared by all backends.
//
// Contract: a successful Write durably records the entry before the caller
// proceeds, and the background processor reapplies pending entries to the
// database (idempotently, via insertCallback) so nothing is lost across a
// crash. The "file" backend is single-instance; the "redis" backend uses a
// Redis Streams consumer group so multiple API instances share the work.
type WAL interface {
	// Write durably records a log entry. Must be fast and reliable.
	Write(log *models.Log) error

	// StartProcessor starts the background processor that drains pending
	// entries into the database via insertCallback. Calling it twice is a no-op.
	StartProcessor(insertCallback func(*models.Log) error)

	// StopProcessor gracefully stops the background processor.
	StopProcessor()

	// ProcessPendingLogs drains the currently-pending entries once. Backs the
	// /wal/force-process endpoint.
	ProcessPendingLogs() error

	// GetStats returns a snapshot of WAL statistics.
	GetStats() WALStats
}

// Compile-time guarantee that the file backend satisfies the interface.
var _ WAL = (*WriteAheadLog)(nil)

// NoopWAL is a no-op backend used when durability is disabled or could not be
// initialized. It satisfies WAL so callers never need nil checks.
type NoopWAL struct{}

var _ WAL = NoopWAL{}

func (NoopWAL) Write(*models.Log) error                { return nil }
func (NoopWAL) StartProcessor(func(*models.Log) error) {}
func (NoopWAL) StopProcessor()                         {}
func (NoopWAL) ProcessPendingLogs() error              { return nil }
func (NoopWAL) GetStats() WALStats                     { return WALStats{} }
