package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/gin-gonic/gin"
)

// MerkleHandler handles Merkle tree and batch-related HTTP requests
type MerkleHandler struct {
	batchProcessor *merkle.BatchProcessor
	cache          *cache.RedisCache
}

// NewMerkleHandler creates a new Merkle handler
func NewMerkleHandler(batchProcessor *merkle.BatchProcessor, cache *cache.RedisCache) *MerkleHandler {
	return &MerkleHandler{
		batchProcessor: batchProcessor,
		cache:          cache,
	}
}

// CreateBatch handles POST /merkle/batch
// @Summary Create a new Merkle batch
// @Description Process pending logs into a Merkle tree batch
// @Tags Merkle
// @Accept json
// @Produce json
// @Param request body models.CreateBatchRequest false "Batch configuration"
// @Success 202 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /merkle/batch [post]
func (h *MerkleHandler) CreateBatch(c *gin.Context) {
	var req models.CreateBatchRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		// If binding fails, use default batch size
		req.BatchSize = 100
	}

	// Validate batch size
	if req.BatchSize <= 0 || req.BatchSize > 1000 {
		req.BatchSize = 100
	}

	// Submit batch job
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.batchProcessor.ProcessBatch(ctx, req.BatchSize); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "batch_error",
			Message: fmt.Sprintf("Failed to submit batch: %v", err),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusAccepted, models.SuccessResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("Batch job submitted with size %d", req.BatchSize),
		Data: gin.H{
			"batch_size": req.BatchSize,
			"status":     "processing",
		},
	})
}

// GetBatch handles GET /merkle/batch/:id
// @Summary Get batch details
// @Description Retrieve a specific batch with its logs
// @Tags Merkle
// @Produce json
// @Param id path string true "Batch ID"
// @Success 200 {object} models.GetBatchResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /merkle/batch/{id} [get]
func (h *MerkleHandler) GetBatch(c *gin.Context) {
	batchID := c.Param("id")

	if batchID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Batch ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Build cache key
	cacheKey := cache.BuildBatchKey(batchID)

	// Try cache first
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if h.cache.Enabled {
		var cachedBatch models.GetBatchResponse
		if err := h.cache.GetJSON(ctx, cacheKey, &cachedBatch); err == nil {
			c.Header("X-Cache", "HIT")
			c.JSON(http.StatusOK, cachedBatch)
			return
		}
	}

	// Get batch
	batch, err := h.batchProcessor.GetBatch(ctx, batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
			Code:    http.StatusNotFound,
		})
		return
	}

	// Cache batch
	if h.cache.Enabled {
		h.cache.SetJSON(ctx, cacheKey, batch, 1*time.Hour)
	}

	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, batch)
}

// VerifyBatch handles POST /merkle/verify/:id
// @Summary Verify batch integrity
// @Description Verify the integrity of a batch by recalculating its Merkle root
// @Tags Merkle
// @Produce json
// @Param id path string true "Batch ID"
// @Success 200 {object} models.VerifyBatchResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /merkle/verify/{id} [post]
func (h *MerkleHandler) VerifyBatch(c *gin.Context) {
	batchID := c.Param("id")

	if batchID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Batch ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Verify batch
	result, err := h.batchProcessor.VerifyBatch(ctx, batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
			Code:    http.StatusNotFound,
		})
		return
	}

	// Set appropriate status code based on verification result
	statusCode := http.StatusOK
	if !result.IsValid {
		statusCode = http.StatusConflict // 409 Conflict for integrity violation
	}

	c.JSON(statusCode, result)
}

// ListBatches handles GET /merkle/batches
// @Summary List all batches
// @Description Get a list of all Merkle batches
// @Tags Merkle
// @Produce json
// @Param limit query int false "Limit results" default(50)
// @Param offset query int false "Offset for pagination" default(0)
// @Success 200 {object} models.ListBatchesResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /merkle/batches [get]
func (h *MerkleHandler) ListBatches(c *gin.Context) {
	// Parse query parameters
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// List batches
	response, err := h.batchProcessor.ListBatches(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "database_error",
			Message: "Failed to list batches",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Apply pagination (simple slice-based for now)
	if offset < len(response.Batches) {
		end := offset + limit
		if end > len(response.Batches) {
			end = len(response.Batches)
		}
		response.Batches = response.Batches[offset:end]
	} else {
		response.Batches = []*models.BatchInfo{}
	}

	c.JSON(http.StatusOK, response)
}

// GetBatchStats handles GET /merkle/stats
// @Summary Get batch statistics
// @Description Get statistics about batch processing
// @Tags Merkle
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /merkle/stats [get]
func (h *MerkleHandler) GetBatchStats(c *gin.Context) {
	stats := h.batchProcessor.GetStats()

	response := gin.H{
		"batch_processor": stats,
		"is_running":      h.batchProcessor.IsRunning(),
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// ForceBatch handles POST /merkle/force-batch
// @Summary Force immediate batch processing
// @Description Immediately process all pending logs into batches
// @Tags Merkle
// @Accept json
// @Produce json
// @Param request body models.CreateBatchRequest false "Batch configuration"
// @Success 202 {object} models.SuccessResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /merkle/force-batch [post]
func (h *MerkleHandler) ForceBatch(c *gin.Context) {
	var req models.CreateBatchRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		req.BatchSize = 100
	}

	if req.BatchSize <= 0 || req.BatchSize > 1000 {
		req.BatchSize = 100
	}

	// Submit multiple batch jobs to process all pending logs
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	jobsSubmitted := 0
	maxJobs := 10 // Process up to 10 batches at once

	for i := 0; i < maxJobs; i++ {
		if err := h.batchProcessor.ProcessBatch(ctx, req.BatchSize); err != nil {
			break
		}
		jobsSubmitted++
	}

	if jobsSubmitted == 0 {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "batch_error",
			Message: "Failed to submit batch jobs",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusAccepted, models.SuccessResponse{
		Status:  "accepted",
		Message: fmt.Sprintf("Submitted %d batch jobs for processing", jobsSubmitted),
		Data: gin.H{
			"jobs_submitted": jobsSubmitted,
			"batch_size":     req.BatchSize,
		},
	})
}
