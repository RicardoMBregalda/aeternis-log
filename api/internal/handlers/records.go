package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/database"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/logger"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/merkle"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// RecordHandler handles generic, domain-scoped record requests under
// /api/v1/{domain}/records. A Log is just a Record in the "logs" domain.
type RecordHandler struct {
	collections    *database.Collections
	batchProcessor *merkle.BatchProcessor
}

// NewRecordHandler creates a new record handler.
func NewRecordHandler(collections *database.Collections, batchProcessor *merkle.BatchProcessor) *RecordHandler {
	return &RecordHandler{collections: collections, batchProcessor: batchProcessor}
}

// tenantFrom returns the caller's tenant (set by the auth middleware), falling
// back to "default" when auth is disabled. All record operations are scoped to it.
func tenantFrom(c *gin.Context) string {
	if t := c.GetString("tenant"); t != "" {
		return t
	}
	return "default"
}

// CreateRecord handles POST /api/v1/{domain}/records
// @Summary Create a record
// @Description Create a domain-scoped auditable record with an integrity hash
// @Tags Records
// @Accept json
// @Produce json
// @Param domain path string true "Domain"
// @Param record body models.CreateRecordRequest true "Record data"
// @Success 201 {object} models.SuccessResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/{domain}/records [post]
func (h *RecordHandler) CreateRecord(c *gin.Context) {
	log := logger.WithRequestID(c.GetString("request_id"))
	domain := c.Param("domain")

	var req models.CreateRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	if req.Timestamp == "" {
		req.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	record := &models.Record{
		Tenant:      tenantFrom(c),
		Domain:      domain,
		ID:          req.ID,
		Timestamp:   req.Timestamp,
		Source:      req.Source,
		Payload:     req.Payload,
		HashFields:  req.HashFields,
		HashVersion: models.CurrentHashVersion,
		CreatedAt:   models.FlexTime{Time: time.Now().UTC()},
	}
	record.Hash = record.CalculateHash()

	if err := record.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := h.collections.InsertRecord(ctx, record); err != nil {
		if errors.Is(err, database.ErrDuplicateRecord) {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "conflict",
				Message: fmt.Sprintf("Record already exists: %s/%s", domain, record.ID),
				Code:    http.StatusConflict,
			})
			return
		}
		log.Error().Err(err).Str("domain", domain).Str("record_id", record.ID).Msg("failed to insert record")
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "database_error",
			Message: "Failed to create record",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, models.SuccessResponse{
		Status:  "success",
		Message: "Record created successfully",
		Data: gin.H{
			"domain": domain,
			"id":     record.ID,
			"hash":   record.Hash,
		},
	})
}

// ListRecords handles GET /api/v1/{domain}/records
// @Summary List records
// @Description List domain records with filters and keyset (cursor) pagination
// @Tags Records
// @Produce json
// @Param domain path string true "Domain"
// @Param source query string false "Filter by source"
// @Param limit query int false "Limit results" default(50)
// @Param offset query int false "Offset for pagination" default(0)
// @Param cursor query string false "Opaque keyset cursor; when set, overrides offset"
// @Success 200 {object} models.ListRecordsResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/{domain}/records [get]
func (h *RecordHandler) ListRecords(c *gin.Context) {
	domain := c.Param("domain")
	source := c.Query("source")
	cursorStr := c.Query("cursor")

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Base filter (tenant + domain-scoped; soft-deleted records hidden).
	baseFilter := bson.M{"tenant": tenantFrom(c), "domain": domain, "deleted_at": bson.M{"$exists": false}}
	if source != "" {
		baseFilter["source"] = source
	}

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

	findOptions := database.NewFindOptions().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "id", Value: -1}})
	if cursorStr == "" {
		findOptions.SetSkip(int64(offset))
	}

	records, err := h.collections.FindRecords(ctx, queryFilter, findOptions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve records",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	total, err := h.collections.CountRecords(ctx, baseFilter)
	if err != nil {
		total = int64(len(records))
	}

	response := models.ListRecordsResponse{
		Records: records,
		Total:   int(total),
		Limit:   limit,
		Offset:  offset,
	}
	if len(records) == limit {
		last := records[len(records)-1]
		response.NextCursor = encodeCursor(last.CreatedAt.Time, last.ID)
	}

	c.JSON(http.StatusOK, response)
}

// GetRecord handles GET /api/v1/{domain}/records/:id
// @Summary Get a record by ID
// @Tags Records
// @Produce json
// @Param domain path string true "Domain"
// @Param id path string true "Record ID"
// @Success 200 {object} models.Record
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/{domain}/records/{id} [get]
func (h *RecordHandler) GetRecord(c *gin.Context) {
	domain := c.Param("domain")
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	record, err := h.collections.FindRecordByID(ctx, tenantFrom(c), domain, id)
	if err != nil || record.DeletedAt != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: fmt.Sprintf("Record not found: %s/%s", domain, id),
			Code:    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, record)
}

// DeleteRecord handles DELETE /api/v1/{domain}/records/:id
// @Summary Soft-delete a record
// @Description Soft-delete a record (sets deleted_at; document and anchor preserved)
// @Tags Records
// @Produce json
// @Param domain path string true "Domain"
// @Param id path string true "Record ID"
// @Success 200 {object} models.SuccessResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/{domain}/records/{id} [delete]
func (h *RecordHandler) DeleteRecord(c *gin.Context) {
	domain := c.Param("domain")
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	tenant := tenantFrom(c)
	existing, err := h.collections.FindRecordByID(ctx, tenant, domain, id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: fmt.Sprintf("Record not found: %s/%s", domain, id),
			Code:    http.StatusNotFound,
		})
		return
	}

	if existing.DeletedAt == nil {
		if err := h.collections.SoftDeleteRecord(ctx, tenant, domain, id); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "database_error",
				Message: "Failed to delete record",
				Code:    http.StatusInternalServerError,
			})
			return
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status:  "success",
		Message: "Record soft-deleted (audit trail preserved)",
		Data:    gin.H{"domain": domain, "id": id, "soft_deleted": true},
	})
}

// ForceRecordBatch handles POST /api/v1/{domain}/records/batch
// @Summary Batch and anchor pending records
// @Description Batch up to batch_size pending records in the domain, compute the Merkle root and anchor it to Fabric
// @Tags Records
// @Accept json
// @Produce json
// @Param domain path string true "Domain"
// @Param request body models.CreateBatchRequest false "Batch configuration"
// @Success 200 {object} models.RecordBatchResult
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/{domain}/records/batch [post]
func (h *RecordHandler) ForceRecordBatch(c *gin.Context) {
	domain := c.Param("domain")

	var req models.CreateBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.BatchSize = 100
	}
	if req.BatchSize <= 0 || req.BatchSize > 1000 {
		req.BatchSize = 100
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()

	result, err := h.batchProcessor.ProcessRecordBatch(ctx, tenantFrom(c), domain, req.BatchSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "batch_error",
			Message: err.Error(),
			Code:    http.StatusInternalServerError,
		})
		return
	}
	if result == nil {
		c.JSON(http.StatusOK, models.SuccessResponse{
			Status:  "success",
			Message: "No pending records to batch",
			Data:    gin.H{"domain": domain, "num_records": 0},
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// VerifyRecordBatch handles POST /api/v1/{domain}/records/verify/:batchId
// @Summary Verify a record batch's integrity
// @Description Recompute the batch Merkle root and compare it with the anchored one
// @Tags Records
// @Produce json
// @Param domain path string true "Domain"
// @Param batchId path string true "Batch ID"
// @Success 200 {object} models.VerifyBatchResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.VerifyBatchResponse
// @Router /api/v1/{domain}/records/verify/{batchId} [post]
func (h *RecordHandler) VerifyRecordBatch(c *gin.Context) {
	domain := c.Param("domain")
	batchID := c.Param("batchId")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	result, err := h.batchProcessor.VerifyRecordBatch(ctx, tenantFrom(c), domain, batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: err.Error(),
			Code:    http.StatusNotFound,
		})
		return
	}

	statusCode := http.StatusOK
	if !result.IsValid {
		statusCode = http.StatusConflict // 409 for integrity violation
	}
	c.JSON(statusCode, result)
}
