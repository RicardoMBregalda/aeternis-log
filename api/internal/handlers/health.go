package handlers

import (
	"net/http"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/database"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	mongoClient    *database.MongoClient
	collections    *database.Collections
	cache          *cache.RedisCache
	fabricClient   *fabric.FabricClient
	batchProcessor *merkle.BatchProcessor
	version        string
	buildTime      string
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(
	mongoClient *database.MongoClient,
	collections *database.Collections,
	cache *cache.RedisCache,
	fabricClient *fabric.FabricClient,
	batchProcessor *merkle.BatchProcessor,
	version string,
	buildTime string,
) *HealthHandler {
	return &HealthHandler{
		mongoClient:    mongoClient,
		collections:    collections,
		cache:          cache,
		fabricClient:   fabricClient,
		batchProcessor: batchProcessor,
		version:        version,
		buildTime:      buildTime,
	}
}

// HealthCheck handles GET /health
// @Summary Health check
// @Description Check if the API and its dependencies are healthy
// @Tags Health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /health [get]
func (h *HealthHandler) HealthCheck(c *gin.Context) {
	ctx := c.Request.Context()
	
	status := "healthy"
	services := make(map[string]interface{})

	// Check MongoDB
	mongoStatus := "healthy"
	if h.mongoClient != nil {
		if err := h.mongoClient.HealthCheck(ctx); err != nil {
			mongoStatus = "unhealthy: " + err.Error()
			status = "degraded"
		}
	} else {
		mongoStatus = "not configured"
		status = "degraded"
	}
	services["mongodb"] = mongoStatus

	// Check Redis
	redisStatus := "disabled"
	if h.cache != nil && h.cache.Enabled {
		if err := h.cache.HealthCheck(ctx); err != nil {
			redisStatus = "unhealthy: " + err.Error()
			status = "degraded"
		} else {
			redisStatus = "healthy"
		}
	} else if h.cache == nil {
		redisStatus = "not configured"
	}
	services["redis"] = redisStatus

	// Check Fabric
	fabricStatus := "disabled"
	if h.fabricClient != nil {
		if err := h.fabricClient.HealthCheck(ctx); err != nil {
			fabricStatus = "unhealthy: " + err.Error()
			// Don't mark as degraded for Fabric - it's optional
		} else {
			fabricStatus = "healthy"
		}
	}
	services["fabric"] = fabricStatus

	// Check Batch Processor
	batchStatus := "stopped"
	if h.batchProcessor != nil && h.batchProcessor.IsRunning() {
		batchStatus = "running"
	}
	services["batch_processor"] = batchStatus

	response := gin.H{
		"status":     status,
		"version":    h.version,
		"build_time": h.buildTime,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"services":   services,
	}

	c.JSON(http.StatusOK, response)
}

// GetStats handles GET /stats
// @Summary Get system statistics
// @Description Get comprehensive system statistics
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /stats [get]
func (h *HealthHandler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()
	
	stats := gin.H{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   h.version,
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

	// Sync stats
	if h.collections != nil {
		if syncStats, err := h.collections.AggregateSyncStats(ctx); err == nil {
			stats["sync"] = syncStats
		}

		// Total logs count
		if totalLogs, err := h.collections.CountLogs(ctx, map[string]interface{}{}); err == nil {
			stats["total_logs"] = totalLogs
		}
	}

	c.JSON(http.StatusOK, stats)
}
