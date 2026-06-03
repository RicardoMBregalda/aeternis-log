package handlers

import (
	"net/http"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/wal"
	"github.com/gin-gonic/gin"
)

// WALHandler handles WAL-related HTTP requests
type WALHandler struct {
	wal *wal.WriteAheadLog
}

// NewWALHandler creates a new WAL handler
func NewWALHandler(wal *wal.WriteAheadLog) *WALHandler {
	return &WALHandler{
		wal: wal,
	}
}

// GetStats handles GET /wal/stats
// @Summary Get WAL statistics
// @Description Retrieve statistics about the Write-Ahead Log
// @Tags WAL
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /wal/stats [get]
func (h *WALHandler) GetStats(c *gin.Context) {
	if h.wal == nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error:   "wal_not_configured",
			Message: "WAL is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	stats := h.wal.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// ForceProcess handles POST /wal/force-process
// @Summary Force WAL processing
// @Description Force immediate processing of pending WAL entries
// @Tags WAL
// @Produce json
// @Success 200 {object} models.SuccessResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /wal/force-process [post]
func (h *WALHandler) ForceProcess(c *gin.Context) {
	if h.wal == nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error:   "wal_not_configured",
			Message: "WAL is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	// Get stats before processing
	statsBefore := h.wal.GetStats()

	// Process pending logs
	err := h.wal.ProcessPendingLogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "processing_error",
			Message: err.Error(),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Get stats after processing
	statsAfter := h.wal.GetStats()
	processed := statsAfter.TotalProcessed - statsBefore.TotalProcessed

	c.JSON(http.StatusOK, models.SuccessResponse{
		Status:  "success",
		Message: "WAL processing completed",
		Data: gin.H{
			"processed":    processed,
			"stats_before": statsBefore,
			"stats_after":  statsAfter,
		},
	})
}

// GetHealth handles GET /wal/health
// @Summary Check WAL health
// @Description Check if the WAL is healthy and operational
// @Tags WAL
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} models.ErrorResponse
// @Router /wal/health [get]
func (h *WALHandler) GetHealth(c *gin.Context) {
	if h.wal == nil {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
			Error:   "wal_not_configured",
			Message: "WAL is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	stats := h.wal.GetStats()
	
	healthy := true
	issues := []string{}

	// Check if processor is running
	if !stats.ProcessorRunning {
		healthy = false
		issues = append(issues, "processor not running")
	}

	// Check if pending count is too high (> 10000)
	if stats.PendingCount > 10000 {
		healthy = false
		issues = append(issues, "high pending count")
	}

	status := "healthy"
	statusCode := http.StatusOK
	if !healthy {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, gin.H{
		"status": status,
		"healthy": healthy,
		"issues": issues,
		"stats": stats,
	})
}
