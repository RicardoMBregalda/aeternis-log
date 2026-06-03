package wal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
)

// WriteAheadLog implements a Write-Ahead Log for guaranteed durability
// Ensures 0% data loss by persisting logs to disk before acknowledging writes
type WriteAheadLog struct {
	walDir        string
	pendingFile   string
	processedFile string
	checkInterval time.Duration

	writeLock sync.Mutex
	running   bool
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// Callback function to insert log into MongoDB
	insertCallback func(*models.Log) error

	// Statistics
	stats     WALStats
	statsLock sync.RWMutex
}

// WALStats holds statistics about WAL operations
type WALStats struct {
	PendingCount     int   `json:"pending_count"`
	TotalWritten     int64 `json:"total_written"`
	TotalProcessed   int64 `json:"total_processed"`
	ProcessorRunning bool  `json:"processor_running"`
	LastError        string `json:"last_error,omitempty"`
}

// WALEntry represents a single entry in the WAL
type WALEntry struct {
	WALTimestamp string       `json:"wal_timestamp"`
	LogData      *models.Log  `json:"log_data"`
}

// NewWriteAheadLog creates a new WAL instance
func NewWriteAheadLog(walDir string, checkInterval time.Duration) (*WriteAheadLog, error) {
	// Create WAL directory if not exists
	if err := os.MkdirAll(walDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	wal := &WriteAheadLog{
		walDir:        walDir,
		pendingFile:   filepath.Join(walDir, "logs_pending.wal"),
		processedFile: filepath.Join(walDir, "logs_processed.wal"),
		checkInterval: checkInterval,
		stopChan:      make(chan struct{}),
		stats: WALStats{
			ProcessorRunning: false,
		},
	}

	// Recover pending logs count
	if err := wal.recoverPendingLogs(); err != nil {
		return nil, fmt.Errorf("failed to recover pending logs: %w", err)
	}

	return wal, nil
}

// NewWriteAheadLogWithConfig creates a WAL instance from config
func NewWriteAheadLogWithConfig(cfg interface{}, insertCallback func(*models.Log) error) *WriteAheadLog {
	// Try to extract fields from config interface
	var dir string
	var interval time.Duration
	enabled := true
	
	// Use type assertion to get config values
	if cfgMap, ok := cfg.(map[string]interface{}); ok {
		if d, ok := cfgMap["directory"].(string); ok {
			dir = d
		}
		if i, ok := cfgMap["check_interval"].(time.Duration); ok {
			interval = i
		}
		if e, ok := cfgMap["enabled"].(bool); ok {
			enabled = e
		}
	}
	
	// Set defaults
	if dir == "" {
		dir = "/var/log/tcc-wal"
	}
	if interval == 0 {
		interval = 5 * time.Second
	}
	
	wal, err := NewWriteAheadLog(dir, interval)
	if err != nil || !enabled {
		// Return dummy WAL if disabled or error
		return &WriteAheadLog{
			walDir:        dir,
			checkInterval: interval,
			stopChan:      make(chan struct{}),
			running:       false,
		}
	}
	
	wal.insertCallback = insertCallback
	return wal
}

// Write writes a log entry to the WAL
// This operation MUST be fast (< 1ms) and reliable
func (w *WriteAheadLog) Write(log *models.Log) error {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()

	// Create WAL entry with timestamp
	entry := WALEntry{
		WALTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
		LogData:      log,
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		w.updateLastError(fmt.Errorf("failed to marshal WAL entry: %w", err))
		return err
	}

	// Open file in append mode with sync flag
	file, err := os.OpenFile(w.pendingFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY|os.O_SYNC, 0644)
	if err != nil {
		w.updateLastError(fmt.Errorf("failed to open WAL file: %w", err))
		return err
	}
	defer file.Close()

	// Lock file for exclusive access
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		w.updateLastError(fmt.Errorf("failed to lock WAL file: %w", err))
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	// Write entry (one per line)
	if _, err := file.Write(append(data, '\n')); err != nil {
		w.updateLastError(fmt.Errorf("failed to write to WAL: %w", err))
		return err
	}

	// Force sync to disk (critical for durability)
	if err := file.Sync(); err != nil {
		w.updateLastError(fmt.Errorf("failed to sync WAL: %w", err))
		return err
	}

	// Update statistics
	w.statsLock.Lock()
	w.stats.TotalWritten++
	w.stats.PendingCount++
	w.statsLock.Unlock()

	return nil
}

// StartProcessor starts the background processor goroutine
func (w *WriteAheadLog) StartProcessor(insertCallback func(*models.Log) error) {
	if w.running {
		return
	}

	w.insertCallback = insertCallback
	w.running = true

	w.statsLock.Lock()
	w.stats.ProcessorRunning = true
	w.statsLock.Unlock()

	w.wg.Add(1)
	go w.processorLoop()
}

// StopProcessor gracefully stops the processor
func (w *WriteAheadLog) StopProcessor() {
	if !w.running {
		return
	}

	w.running = false
	close(w.stopChan)
	w.wg.Wait()

	w.statsLock.Lock()
	w.stats.ProcessorRunning = false
	w.statsLock.Unlock()
}

// processorLoop is the main processing loop
func (w *WriteAheadLog) processorLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			if err := w.ProcessPendingLogs(); err != nil {
				w.updateLastError(err)
			}
		}
	}
}

// ProcessPendingLogs processes all pending logs in the WAL
func (w *WriteAheadLog) ProcessPendingLogs() error {
	// Check if pending file exists
	if _, err := os.Stat(w.pendingFile); os.IsNotExist(err) {
		return nil
	}

	// Read all pending logs
	pendingLogs, err := w.readPendingLogs()
	if err != nil {
		return fmt.Errorf("failed to read pending logs: %w", err)
	}

	if len(pendingLogs) == 0 {
		return nil
	}

	// Process each log
	var failedEntries []string
	processedCount := 0

	for _, entry := range pendingLogs {
		var walEntry WALEntry
		if err := json.Unmarshal([]byte(entry), &walEntry); err != nil {
			// Invalid JSON, skip this entry
			continue
		}

		// Try to insert into MongoDB
		if err := w.insertCallback(walEntry.LogData); err != nil {
			// Failed, keep in pending
			failedEntries = append(failedEntries, entry)
		} else {
			// Success, record in processed file
			processedCount++
			w.recordProcessed(&walEntry)
		}
	}

	// Update pending file with failed entries
	if err := w.updatePendingFile(failedEntries); err != nil {
		return fmt.Errorf("failed to update pending file: %w", err)
	}

	// Update statistics
	w.statsLock.Lock()
	w.stats.TotalProcessed += int64(processedCount)
	w.stats.PendingCount = len(failedEntries)
	w.statsLock.Unlock()

	return nil
}

// readPendingLogs reads all entries from the pending file
func (w *WriteAheadLog) readPendingLogs() ([]string, error) {
	file, err := os.Open(w.pendingFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			entries = append(entries, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// updatePendingFile rewrites the pending file with remaining entries
func (w *WriteAheadLog) updatePendingFile(entries []string) error {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()

	if len(entries) == 0 {
		// No pending entries, remove file
		return os.Remove(w.pendingFile)
	}

	// Write to temporary file first
	tempFile := w.pendingFile + ".tmp"
	file, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if _, err := file.WriteString(entry + "\n"); err != nil {
			file.Close()
			return err
		}
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	file.Close()

	// Atomic rename
	return os.Rename(tempFile, w.pendingFile)
}

// recordProcessed records a processed log in the processed file
func (w *WriteAheadLog) recordProcessed(entry *WALEntry) error {
	record := map[string]interface{}{
		"wal_timestamp":       entry.WALTimestamp,
		"processed_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"log_id":              entry.LogData.ID,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(w.processedFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(append(data, '\n'))
	return err
}

// recoverPendingLogs counts pending logs at startup
func (w *WriteAheadLog) recoverPendingLogs() error {
	if _, err := os.Stat(w.pendingFile); os.IsNotExist(err) {
		return nil
	}

	entries, err := w.readPendingLogs()
	if err != nil {
		return err
	}

	w.statsLock.Lock()
	w.stats.PendingCount = len(entries)
	w.statsLock.Unlock()

	return nil
}

// GetStats returns current WAL statistics
func (w *WriteAheadLog) GetStats() WALStats {
	w.statsLock.RLock()
	defer w.statsLock.RUnlock()

	stats := w.stats

	// Get file sizes
	if info, err := os.Stat(w.pendingFile); err == nil {
		stats.PendingCount = int(info.Size())
	}

	return stats
}

// updateLastError updates the last error in stats
func (w *WriteAheadLog) updateLastError(err error) {
	if err == nil {
		return
	}

	w.statsLock.Lock()
	w.stats.LastError = err.Error()
	w.statsLock.Unlock()
}

// ClearProcessedHistory removes old processed logs
func (w *WriteAheadLog) ClearProcessedHistory(olderThanDays int) error {
	if _, err := os.Stat(w.processedFile); os.IsNotExist(err) {
		return nil
	}

	cutoffTime := time.Now().UTC().AddDate(0, 0, -olderThanDays)

	file, err := os.Open(w.processedFile)
	if err != nil {
		return err
	}
	defer file.Close()

	tempFile := w.processedFile + ".tmp"
	outFile, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	scanner := bufio.NewScanner(file)
	keptCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}

		// Check processed timestamp
		if tsStr, ok := record["processed_timestamp"].(string); ok {
			if ts, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
				if ts.After(cutoffTime) {
					outFile.WriteString(line + "\n")
					keptCount++
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	outFile.Close()
	file.Close()

	// Replace original file
	return os.Rename(tempFile, w.processedFile)
}

// ForceProcess forces immediate processing of pending logs
func (w *WriteAheadLog) ForceProcess(ctx context.Context) error {
	return w.ProcessPendingLogs()
}

// IsRunning returns whether the processor is running
func (w *WriteAheadLog) IsRunning() bool {
	w.statsLock.RLock()
	defer w.statsLock.RUnlock()
	return w.stats.ProcessorRunning
}
