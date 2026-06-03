package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/database"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/wal"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// StatsHandler handles statistics-related HTTP requests
type StatsHandler struct {
	collections    *database.Collections
	mongoClient    *database.MongoClient
	cache          *cache.RedisCache
	fabricClient   *fabric.FabricClient
	batchProcessor *merkle.BatchProcessor
	wal            *wal.WriteAheadLog
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(
	collections *database.Collections,
	mongoClient *database.MongoClient,
	cache *cache.RedisCache,
	fabricClient *fabric.FabricClient,
	batchProcessor *merkle.BatchProcessor,
	wal *wal.WriteAheadLog,
) *StatsHandler {
	return &StatsHandler{
		collections:    collections,
		mongoClient:    mongoClient,
		cache:          cache,
		fabricClient:   fabricClient,
		batchProcessor: batchProcessor,
		wal:            wal,
	}
}

// GetStats handles GET /stats
// @Summary Get system statistics
// @Description Retrieve comprehensive system statistics including logs, sync status, and components
// @Tags Stats
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /stats [get]
func (h *StatsHandler) GetStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	stats := gin.H{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Total logs count
	if h.collections != nil {
		if totalLogs, err := h.collections.CountLogs(ctx, bson.M{}); err == nil {
			stats["total_logs"] = totalLogs
		}

		// Logs by level
		levelCounts := make(map[string]int64)
		for _, level := range []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"} {
			if count, err := h.collections.CountLogs(ctx, bson.M{"level": level}); err == nil {
				levelCounts[level] = count
			}
		}
		stats["logs_by_level"] = levelCounts

		// Sync statistics
		if syncStats, err := h.collections.AggregateSyncStats(ctx); err == nil {
			stats["sync"] = syncStats
		}
	}

	// MongoDB stats
	if h.mongoClient != nil {
		if mongoStats, err := h.mongoClient.GetStats(ctx); err == nil {
			stats["mongodb"] = mongoStats
		}
	}

	// Redis stats
	if h.cache != nil && h.cache.Enabled {
		if redisStats, err := h.cache.GetStats(ctx); err == nil {
			stats["redis"] = redisStats
		}
	}

	// Fabric stats
	if h.fabricClient != nil {
		stats["fabric"] = h.fabricClient.GetStats()
	}

	// Batch processor stats
	if h.batchProcessor != nil {
		stats["batch_processor"] = h.batchProcessor.GetStats()
	}

	// WAL stats
	if h.wal != nil {
		stats["wal"] = h.wal.GetStats()
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetLogStats handles GET /stats/logs
// @Summary Get detailed log statistics
// @Description Retrieve detailed statistics about logs including distribution by source, level, and time
// @Tags Stats
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /stats/logs [get]
func (h *StatsHandler) GetLogStats(c *gin.Context) {
	if h.collections == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	stats := gin.H{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Total logs
	if totalLogs, err := h.collections.CountLogs(ctx, bson.M{}); err == nil {
		stats["total_logs"] = totalLogs
	}

	// Logs by level
	levelCounts := make(map[string]int64)
	for _, level := range []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"} {
		if count, err := h.collections.CountLogs(ctx, bson.M{"level": level}); err == nil {
			levelCounts[level] = count
		}
	}
	stats["logs_by_level"] = levelCounts

	// Logs by source (top 10)
	// Note: In production, you'd use an aggregation pipeline
	stats["note"] = "For detailed source statistics, use MongoDB aggregation directly"

	// Recent logs count (last 24 hours)
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	if recentCount, err := h.collections.CountLogs(ctx, bson.M{
		"created_at": bson.M{"$gte": yesterday},
	}); err == nil {
		stats["logs_last_24h"] = recentCount
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetSyncStats handles GET /stats/sync
// @Summary Get sync statistics
// @Description Retrieve statistics about sync status between MongoDB and Fabric
// @Tags Stats
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /stats/sync [get]
func (h *StatsHandler) GetSyncStats(c *gin.Context) {
	if h.collections == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	syncStats, err := h.collections.AggregateSyncStats(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get sync stats",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"data":      syncStats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
