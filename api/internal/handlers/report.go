package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/database"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/report"
	"github.com/gin-gonic/gin"
)

// ReportHandler produces audit reports (JSON and PDF) of anchored batches for a
// tenant/domain over an optional time range.
type ReportHandler struct {
	collections *database.Collections
}

// NewReportHandler creates a report handler.
func NewReportHandler(collections *database.Collections) *ReportHandler {
	return &ReportHandler{collections: collections}
}

func (h *ReportHandler) build(c *gin.Context) (models.AuditReport, error) {
	domain := c.Param("domain")
	tenant := tenantFrom(c)
	from := c.Query("from") // RFC3339; empty = open
	to := c.Query("to")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	batches, err := h.collections.AggregateRecordBatches(ctx, tenant, domain, from, to)
	if err != nil {
		return models.AuditReport{}, err
	}
	total := 0
	for _, b := range batches {
		total += b.NumRecords
	}
	return models.AuditReport{
		Tenant:       tenant,
		Domain:       domain,
		From:         from,
		To:           to,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Batches:      batches,
		TotalBatches: len(batches),
		TotalRecords: total,
	}, nil
}

// AuditReportJSON handles GET /api/v1/{domain}/report
// @Summary Audit report (JSON)
// @Tags Reports
// @Produce json
// @Param domain path string true "Domain"
// @Param from query string false "Start (RFC3339)"
// @Param to query string false "End (RFC3339)"
// @Success 200 {object} models.AuditReport
// @Router /api/v1/{domain}/report [get]
func (h *ReportHandler) AuditReportJSON(c *gin.Context) {
	rep, err := h.build(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "report_error", Message: "failed to build the report", Code: http.StatusInternalServerError,
		})
		return
	}
	c.JSON(http.StatusOK, rep)
}

// AuditReportPDF handles GET /api/v1/{domain}/report.pdf
// @Summary Audit report (PDF)
// @Tags Reports
// @Produce application/pdf
// @Param domain path string true "Domain"
// @Param from query string false "Start (RFC3339)"
// @Param to query string false "End (RFC3339)"
// @Success 200 {file} binary
// @Router /api/v1/{domain}/report.pdf [get]
func (h *ReportHandler) AuditReportPDF(c *gin.Context) {
	rep, err := h.build(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "report_error", Message: "failed to build the report", Code: http.StatusInternalServerError,
		})
		return
	}
	pdf, err := report.BuildAuditReportPDF(rep)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "pdf_error", Message: "failed to render the PDF", Code: http.StatusInternalServerError,
		})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q",
		fmt.Sprintf("audit-%s-%s.pdf", rep.Tenant, rep.Domain)))
	c.Data(http.StatusOK, "application/pdf", pdf)
}
