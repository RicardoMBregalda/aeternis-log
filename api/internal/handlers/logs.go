package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/cache"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/database"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/logger"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/wal"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// LogHandler handles log-related HTTP requests
type LogHandler struct {
	collections *database.Collections
	cache       *cache.RedisCache
	wal         wal.WAL
}

// NewLogHandler creates a new log handler
func NewLogHandler(collections *database.Collections, cache *cache.RedisCache, wal wal.WAL) *LogHandler {
	return &LogHandler{
		collections: collections,
		cache:       cache,
		wal:         wal,
	}
}

// CreateLog handles POST /logs
// @Summary Create a new log
// @Description Create a new log entry with automatic hash calculation
// @Tags Logs
// @Accept json
// @Produce json
// @Param log body models.CreateLogRequest true "Log data"
// @Success 201 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /logs [post]
func (h *LogHandler) CreateLog(c *gin.Context) {
	log := logger.WithRequestID(c.GetString("request_id"))

	var req models.CreateLogRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Generate ID if not provided
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if req.Timestamp == "" {
		req.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Create log from request
	logEntry := &models.Log{
		ID:         req.ID,
		Timestamp:  req.Timestamp,
		Source:     req.Source,
		Level:      req.Level,
		Message:    req.Message,
		Metadata:   req.Metadata,
		Stacktrace: req.Stacktrace,
		CreatedAt:  models.FlexTime{Time: time.Now().UTC()},
	}

	// Calculate hash
	logEntry.Hash = logEntry.CalculateHash()

	// Validate log
	if err := logEntry.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Write to WAL first (zero data loss guarantee)
	if h.wal != nil {
		if err := h.wal.Write(logEntry); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "wal_error",
				Message: "Failed to write to WAL",
				Code:    http.StatusInternalServerError,
			})
			return
		}
	}

	// Insert into database
	if err := h.collections.InsertLog(ctx, logEntry); err != nil {
		log.Error().Err(err).Str("log_id", logEntry.ID).Msg("failed to insert log")
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "database_error",
			Message: "Failed to create log",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Create sync control
	syncControl := models.NewSyncControl(logEntry.ID, models.SyncStatusPendingBatch)
	if err := h.collections.InsertSyncControl(ctx, syncControl); err != nil {
		// Log error but don't fail the request: the WAL/batch processor will
		// reconcile sync state, so a missing control record is non-fatal.
		log.Warn().Err(err).Str("log_id", logEntry.ID).Msg("failed to create sync control")
	}

	// Invalidate cache
	if h.cache.Enabled {
		h.cache.InvalidateLogCache(ctx, logEntry.Source)
	}

	c.JSON(http.StatusCreated, models.SuccessResponse{
		Status:  "success",
		Message: "Log created successfully",
		Data: gin.H{
			"id":   logEntry.ID,
			"hash": logEntry.Hash,
		},
	})
}

// GetLogs handles GET /logs
// @Summary List logs
// @Description Get logs with optional filters and pagination
// @Tags Logs
// @Produce json
// @Param source query string false "Filter by source"
// @Param level query string false "Filter by level"
// @Param limit query int false "Limit results" default(50)
// @Param offset query int false "Offset for pagination" default(0)
// @Param cursor query string false "Opaque keyset cursor; when set, overrides offset"
// @Success 200 {object} models.ListLogsResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /logs [get]
func (h *LogHandler) GetLogs(c *gin.Context) {
	// Parse query parameters
	source := c.Query("source")
	level := c.Query("level")
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	cursorStr := c.Query("cursor")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Cache key: cursor and offset pages key separately.
	cacheKey := cache.BuildLogListKey(source, level, limit, offset)
	if cursorStr != "" {
		cacheKey = cache.BuildLogListCursorKey(source, level, limit, cursorStr)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if h.cache.Enabled {
		var cachedResponse models.ListLogsResponse
		if err := h.cache.GetJSON(ctx, cacheKey, &cachedResponse); err == nil {
			c.Header("X-Cache", "HIT")
			c.JSON(http.StatusOK, cachedResponse)
			return
		}
	}

	// Base filter (soft-deleted logs are hidden). Used for the total count
	// regardless of pagination mode.
	baseFilter := bson.M{"deleted_at": bson.M{"$exists": false}}
	if source != "" {
		baseFilter["source"] = source
	}
	if level != "" {
		baseFilter["level"] = level
	}

	// Query filter: in cursor mode, narrow to items strictly past the cursor
	// position under the (created_at desc, id desc) order.
	queryFilter := baseFilter
	if cursorStr != "" {
		cur, err := decodeCursor(cursorStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_cursor",
				Message: "Invalid pagination cursor",
				Code:    http.StatusBadRequest,
			})
			return
		}
		boundary := primitive.NewDateTimeFromTime(time.UnixMilli(cur.TimeMillis).UTC())
		queryFilter = cloneFilter(baseFilter)
		queryFilter["$or"] = bson.A{
			bson.M{"created_at": bson.M{"$lt": boundary}},
			bson.M{"created_at": boundary, "id": bson.M{"$lt": cur.ID}},
		}
	}

	// Sort by (created_at desc, id desc) for a stable keyset order. Offset mode
	// keeps SetSkip; cursor mode relies on the range filter instead.
	findOptions := database.NewFindOptions().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "id", Value: -1}})
	if cursorStr == "" {
		findOptions.SetSkip(int64(offset))
	}

	logs, err := h.collections.FindLogs(ctx, queryFilter, findOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve logs",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	total, err := h.collections.CountLogs(ctx, baseFilter)
	if err != nil {
		total = int64(len(logs))
	}

	response := models.ListLogsResponse{
		Logs:   logs,
		Total:  int(total),
		Limit:  limit,
		Offset: offset,
	}
	// A full page implies there may be more; expose the next cursor.
	if len(logs) == limit {
		last := logs[len(logs)-1]
		response.NextCursor = encodeCursor(last.CreatedAt.Time, last.ID)
	}

	if h.cache.Enabled {
		h.cache.SetJSON(ctx, cacheKey, response, 10*time.Minute)
	}

	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, response)
}

// cloneFilter returns a shallow copy of a bson.M filter so cursor predicates can
// be added without mutating the base filter used for the total count.
func cloneFilter(m bson.M) bson.M {
	out := make(bson.M, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GetLogByID handles GET /logs/:id
// @Summary Get log by ID
// @Description Retrieve a specific log by its ID
// @Tags Logs
// @Produce json
// @Param id path string true "Log ID"
// @Success 200 {object} models.Log
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /logs/{id} [get]
func (h *LogHandler) GetLogByID(c *gin.Context) {
	logID := c.Param("id")

	if logID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Log ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Build cache key
	cacheKey := cache.BuildLogKey(logID)

	// Try cache first
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if h.cache.Enabled {
		var cachedLog models.Log
		if err := h.cache.GetJSON(ctx, cacheKey, &cachedLog); err == nil {
			c.Header("X-Cache", "HIT")
			c.JSON(http.StatusOK, cachedLog)
			return
		}
	}

	// Find log
	log, err := h.collections.FindLogByID(ctx, logID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: fmt.Sprintf("Log not found: %s", logID),
			Code:    http.StatusNotFound,
		})
		return
	}

	// Hide soft-deleted logs from the read endpoint (still preserved for audit).
	if log.DeletedAt != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: fmt.Sprintf("Log not found: %s", logID),
			Code:    http.StatusNotFound,
		})
		return
	}

	// Cache log
	if h.cache.Enabled {
		h.cache.SetJSON(ctx, cacheKey, log, 30*time.Minute)
	}

	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, log)
}

// DeleteLog handles DELETE /logs/:id
// @Summary Soft-delete a log
// @Description Soft-delete a log by ID (sets deleted_at; the document and its blockchain anchor are preserved)
// @Tags Logs
// @Produce json
// @Param id path string true "Log ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /logs/{id} [delete]
func (h *LogHandler) DeleteLog(c *gin.Context) {
	logID := c.Param("id")

	if logID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Log ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Confirm the log exists (and capture its source for cache invalidation).
	existing, err := h.collections.FindLogByID(ctx, logID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: fmt.Sprintf("Log not found: %s", logID),
			Code:    http.StatusNotFound,
		})
		return
	}

	// Soft delete: set deleted_at, keeping the document and its on-chain Merkle
	// anchor intact. Idempotent if the log was already deleted.
	if existing.DeletedAt == nil {
		if err := h.collections.SoftDeleteLog(ctx, logID); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "database_error",
				Message: "Failed to delete log",
				Code:    http.StatusInternalServerError,
			})
			return
		}
	}

	// Invalidate caches so the log no longer appears in reads/listings.
	if h.cache.Enabled {
		h.cache.Delete(ctx, cache.BuildLogKey(logID))
		h.cache.InvalidateLogCache(ctx, existing.Source)
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status:  "success",
		Message: "Log soft-deleted (audit trail and blockchain anchor preserved)",
		Data: gin.H{
			"id":           logID,
			"soft_deleted": true,
		},
	})
}
