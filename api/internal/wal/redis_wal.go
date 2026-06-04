package wal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/go-redis/redis/v8"
)

// nonBlocking is passed as the block duration to perform a single, immediate
// drain (XREADGROUP without BLOCK) instead of waiting for new entries.
const nonBlocking = time.Duration(-1)

// streamField is the single field name used inside each stream entry.
const streamField = "entry"

// RedisWAL is a Write-Ahead Log backed by a Redis Stream and a consumer group.
//
// Each API instance appends entries with XADD and consumes them through a
// shared consumer group, so an entry is delivered to exactly one instance.
// Entries are acknowledged (and deleted) only after a successful insert; an
// instance that dies mid-processing leaves its entries pending, and another
// instance reclaims them via XAUTOCLAIM. This is what allows the API to scale
// horizontally while preserving the WAL's recovery guarantee.
//
// Durability of Write depends on Redis persistence: run Redis with AOF enabled
// (appendfsync always for parity with the file backend's per-write fsync).
type RedisWAL struct {
	client       *redis.Client
	streamKey    string
	group        string
	consumer     string
	readCount    int64
	block        time.Duration
	claimMinIdle time.Duration

	insertCallback func(*models.Log) error

	mu         sync.Mutex
	running    bool
	stopChan   chan struct{}
	procCtx    context.Context
	procCancel context.CancelFunc
	wg         sync.WaitGroup

	stats     WALStats
	statsLock sync.RWMutex
}

// NewRedisWAL builds a Redis Streams WAL from configuration.
func NewRedisWAL(client *redis.Client, cfg *config.WALConfig) *RedisWAL {
	readCount := cfg.ReadCount
	if readCount <= 0 {
		readCount = 128
	}
	block := cfg.BlockTimeout
	if block <= 0 {
		block = 2 * time.Second
	}
	claimMinIdle := cfg.ClaimMinIdle
	if claimMinIdle <= 0 {
		claimMinIdle = 30 * time.Second
	}

	return &RedisWAL{
		client:       client,
		streamKey:    cfg.StreamKey,
		group:        cfg.ConsumerGroup,
		consumer:     consumerName(),
		readCount:    readCount,
		block:        block,
		claimMinIdle: claimMinIdle,
		stopChan:     make(chan struct{}),
	}
}

// consumerName derives a stable, instance-unique consumer name.
func consumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// opCtx returns a short-lived context for one-off Redis operations. It is
// independent of the processor context so acknowledgements still complete
// while the read loop is being cancelled during shutdown.
func (r *RedisWAL) opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// ensureGroup creates the stream and consumer group if they do not exist.
func (r *RedisWAL) ensureGroup(ctx context.Context) error {
	// "0" lets a freshly-created group also pick up entries that were XADDed
	// before the group existed. MKSTREAM creates the stream on demand.
	err := r.client.XGroupCreateMkStream(ctx, r.streamKey, r.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// Write appends a log entry to the stream.
func (r *RedisWAL) Write(log *models.Log) error {
	entry := WALEntry{
		WALTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
		LogData:      log,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		r.updateLastError(fmt.Errorf("failed to marshal WAL entry: %w", err))
		return err
	}

	ctx, cancel := r.opCtx()
	defer cancel()

	// XADD creates the stream if needed and persists per Redis AOF config.
	if err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamKey,
		Values: map[string]interface{}{streamField: data},
	}).Err(); err != nil {
		r.updateLastError(fmt.Errorf("failed to XADD WAL entry: %w", err))
		return err
	}

	r.statsLock.Lock()
	r.stats.TotalWritten++
	r.statsLock.Unlock()
	return nil
}

// StartProcessor starts the background consumer loop.
func (r *RedisWAL) StartProcessor(insertCallback func(*models.Log) error) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.insertCallback = insertCallback
	r.running = true
	r.procCtx, r.procCancel = context.WithCancel(context.Background())
	r.mu.Unlock()

	if err := r.ensureGroup(r.procCtx); err != nil {
		r.updateLastError(fmt.Errorf("failed to create consumer group: %w", err))
	}

	r.statsLock.Lock()
	r.stats.ProcessorRunning = true
	r.statsLock.Unlock()

	r.wg.Add(1)
	go r.processorLoop()
}

// StopProcessor cancels the read loop and waits for it to drain.
func (r *RedisWAL) StopProcessor() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	cancel := r.procCancel
	r.mu.Unlock()

	close(r.stopChan)
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()

	r.statsLock.Lock()
	r.stats.ProcessorRunning = false
	r.statsLock.Unlock()
}

func (r *RedisWAL) processorLoop() {
	defer r.wg.Done()

	for {
		select {
		case <-r.stopChan:
			return
		default:
		}

		// 1. Retry entries already delivered to this consumer but not yet acked
		//    (e.g. a transient insert failure on a previous pass). Reading id
		//    "0" returns this consumer's pending list (PEL).
		if _, err := r.readAndProcess(r.procCtx, "0", nonBlocking); err != nil && err != redis.Nil {
			if r.procCtx.Err() != nil {
				return
			}
			r.updateLastError(err)
		}

		// 2. Reclaim entries abandoned by dead consumers.
		r.claimStale()

		// 3. Block for new entries. The block window also paces retries above.
		if _, err := r.readAndProcess(r.procCtx, ">", r.block); err != nil {
			if r.procCtx.Err() != nil {
				return // context cancelled during shutdown
			}
			r.updateLastError(err)
			// Back off briefly so a persistent error does not hot-loop.
			select {
			case <-r.stopChan:
				return
			case <-time.After(time.Second):
			}
		}
	}
}

// ProcessPendingLogs drains everything currently available without blocking.
func (r *RedisWAL) ProcessPendingLogs() error {
	ctx, cancel := r.opCtx()
	defer cancel()

	if err := r.ensureGroup(ctx); err != nil {
		return err
	}
	r.claimStale()

	// Retry this consumer's own pending entries first.
	if _, err := r.readAndProcess(ctx, "0", nonBlocking); err != nil && err != redis.Nil {
		return err
	}

	// Then drain all newly-available entries.
	for {
		n, err := r.readAndProcess(ctx, ">", nonBlocking)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

// readAndProcess reads one batch from the given id (">" for new entries) and
// processes it. block < 0 means non-blocking.
func (r *RedisWAL) readAndProcess(ctx context.Context, id string, block time.Duration) (int, error) {
	res, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.group,
		Consumer: r.consumer,
		Streams:  []string{r.streamKey, id},
		Count:    r.readCount,
		Block:    block,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // nothing available within the window
		}
		return 0, err
	}

	processed := 0
	for _, stream := range res {
		for _, msg := range stream.Messages {
			if r.handleMessage(msg) {
				processed++
			}
		}
	}
	return processed, nil
}

// claimStale reclaims and processes entries that have been pending (delivered
// but unacknowledged) longer than claimMinIdle, e.g. from a crashed instance.
func (r *RedisWAL) claimStale() {
	ctx, cancel := r.opCtx()
	defer cancel()

	start := "0-0"
	for {
		msgs, next, err := r.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   r.streamKey,
			Group:    r.group,
			Consumer: r.consumer,
			MinIdle:  r.claimMinIdle,
			Start:    start,
			Count:    r.readCount,
		}).Result()
		if err != nil {
			// BUSYGROUP-style/no-group errors are benign before the first read.
			if err != redis.Nil && !strings.Contains(err.Error(), "NOGROUP") {
				r.updateLastError(err)
			}
			return
		}

		for _, msg := range msgs {
			r.handleMessage(msg)
		}

		if next == "0-0" || len(msgs) == 0 {
			return
		}
		start = next
	}
}

// handleMessage inserts one entry and acknowledges it on success. A malformed
// entry is acknowledged and deleted so it cannot block the group (poison pill).
// A failed insert is left pending for retry/reclaim.
func (r *RedisWAL) handleMessage(msg redis.XMessage) bool {
	raw, ok := msg.Values[streamField].(string)
	if !ok {
		r.ackAndDelete(msg.ID)
		return false
	}

	var entry WALEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		r.ackAndDelete(msg.ID)
		return false
	}

	if err := r.insertCallback(entry.LogData); err != nil {
		r.updateLastError(err) // keep pending; will be retried/reclaimed
		return false
	}

	r.ackAndDelete(msg.ID)
	r.statsLock.Lock()
	r.stats.TotalProcessed++
	r.statsLock.Unlock()
	return true
}

// ackAndDelete acknowledges an entry and removes it to keep the stream bounded.
func (r *RedisWAL) ackAndDelete(id string) {
	ctx, cancel := r.opCtx()
	defer cancel()
	r.client.XAck(ctx, r.streamKey, r.group, id)
	r.client.XDel(ctx, r.streamKey, id)
}

// GetStats returns a snapshot, deriving the pending count from the stream
// length (entries are deleted once successfully processed).
func (r *RedisWAL) GetStats() WALStats {
	r.statsLock.RLock()
	stats := r.stats
	r.statsLock.RUnlock()

	ctx, cancel := r.opCtx()
	defer cancel()
	if n, err := r.client.XLen(ctx, r.streamKey).Result(); err == nil {
		stats.PendingCount = int(n)
	}
	return stats
}

func (r *RedisWAL) updateLastError(err error) {
	if err == nil {
		return
	}
	r.statsLock.Lock()
	r.stats.LastError = err.Error()
	r.statsLock.Unlock()
}
