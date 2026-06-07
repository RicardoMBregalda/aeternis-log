package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/gin-gonic/gin"
)

// batchQuerier reads an anchored Merkle batch from the blockchain. *fabric.FabricClient
// satisfies it; the small interface keeps the handler unit-testable.
type batchQuerier interface {
	VerifyMerkleBatch(ctx context.Context, channel, batchID string) (*fabric.QueryResponse, error)
}

// PublicHandler serves unauthenticated, read-only verification endpoints for
// external auditors: confirm that a batch is anchored on-chain and (optionally)
// that a given Merkle root matches the anchored one.
type PublicHandler struct {
	fabric         batchQuerier
	defaultChannel string
}

// NewPublicHandler creates a public verification handler.
func NewPublicHandler(f batchQuerier, defaultChannel string) *PublicHandler {
	return &PublicHandler{fabric: f, defaultChannel: defaultChannel}
}

// GetAnchor handles GET /public/anchors/:batchId
// @Summary Public anchor lookup
// @Description Return the on-chain Merkle batch for a batch id (no authentication).
//             Pass ?root= to also check whether a given Merkle root matches the anchored one.
// @Tags Public
// @Produce json
// @Param batchId path string true "Batch ID"
// @Param channel query string false "Fabric channel (defaults to the server's default channel)"
// @Param root query string false "Merkle root to compare against the anchored one"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} models.ErrorResponse
// @Failure 502 {object} models.ErrorResponse
// @Router /public/anchors/{batchId} [get]
func (h *PublicHandler) GetAnchor(c *gin.Context) {
	batchID := c.Param("batchId")
	channel := c.DefaultQuery("channel", h.defaultChannel)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.fabric.VerifyMerkleBatch(ctx, channel, batchID)
	if err != nil {
		// The chaincode returns "batch <id> does not exist" when nothing is anchored.
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "no anchor found for the given batch id",
				Code:    http.StatusNotFound,
			})
			return
		}
		c.JSON(http.StatusBadGateway, models.ErrorResponse{
			Error:   "fabric_error",
			Message: "failed to query the blockchain",
			Code:    http.StatusBadGateway,
		})
		return
	}

	out := gin.H{
		"anchored": true,
		"channel":  channel,
		"batch":    resp.Data,
	}
	// Optional trustless check: does the caller's root match the anchored one?
	if root := c.Query("root"); root != "" {
		anchored, _ := resp.Data["merkle_root"].(string)
		out["root_matches"] = anchored == root
	}
	c.JSON(http.StatusOK, out)
}
