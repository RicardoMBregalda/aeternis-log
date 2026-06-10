// Package report renders audit reports (PDF) from anchored-batch data.
package report

import (
	"bytes"
	"fmt"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/go-pdf/fpdf"
)

// BuildAuditReportPDF renders an audit report as a PDF: a header with the
// tenant/domain/period and totals, then one block per anchored batch with the full
// batch id, Merkle root and Fabric transaction id. Compression is disabled so the
// content stays greppable (for tests and auditors' tooling).
func BuildAuditReportPDF(r models.AuditReport) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetTitle("Tamper-Evident Audit Report", false)
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, "Tamper-Evident Audit Report", "", 1, "L", false, 0, "")

	meta := func(k, v string) {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(32, 6, k, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.CellFormat(0, 6, v, "", 1, "L", false, 0, "")
	}
	meta("Tenant", r.Tenant)
	meta("Domain", r.Domain)
	meta("Period", fmt.Sprintf("%s  to  %s", orElse(r.From, "(any)"), orElse(r.To, "(now)")))
	meta("Generated", r.GeneratedAt)
	meta("Batches", fmt.Sprintf("%d", r.TotalBatches))
	meta("Records", fmt.Sprintf("%d", r.TotalRecords))
	pdf.Ln(3)

	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	if len(r.Batches) == 0 {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 8, "No anchored batches in this period.", "", 1, "L", false, 0, "")
	}
	for _, b := range r.Batches {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(0, 6, fmt.Sprintf("Batch %s   (records: %d, anchored: %s)",
			b.BatchID, b.NumRecords, b.BatchedAt), "", 1, "L", false, 0, "")
		pdf.SetFont("Courier", "", 8)
		pdf.CellFormat(0, 5, "  merkle root: "+b.MerkleRoot, "", 1, "L", false, 0, "")
		pdf.CellFormat(0, 5, "  fabric tx:   "+orElse(b.TxID, "(not recorded)"), "", 1, "L", false, 0, "")
		pdf.Ln(2)
	}

	pdf.Ln(3)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.MultiCell(0, 4, "Each Merkle root is anchored on Hyperledger Fabric. To verify integrity "+
		"independently, recompute the root from the records (e.g. the `aeternislog` CLI) and compare it with "+
		"the value above and the one anchored on-chain (GET /public/anchors/<batch id>).", "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to render PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
