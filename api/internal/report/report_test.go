package report

import (
	"bytes"
	"testing"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
)

func TestBuildAuditReportPDF(t *testing.T) {
	r := models.AuditReport{
		Tenant:       "acme",
		Domain:       "audit",
		From:         "2026-06-01T00:00:00Z",
		To:           "2026-06-30T00:00:00Z",
		GeneratedAt:  "2026-06-06T00:00:00Z",
		TotalBatches: 1,
		TotalRecords: 3,
		Batches: []models.AuditBatch{
			{
				BatchID:    "acme-audit-abc123",
				MerkleRoot: "deadbeefroot00000000000000000000000000000000000000000000000000ab",
				TxID:       "tx9e5da8fe69be0f99",
				NumRecords: 3,
				BatchedAt:  "2026-06-05T00:00:00Z",
			},
		},
	}
	out, err := BuildAuditReportPDF(r)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF")
	}
	// Compression is disabled, so the report content is present verbatim.
	for _, want := range []string{
		"Tamper-Evident Audit Report", "acme", "audit",
		"acme-audit-abc123", "deadbeefroot00000000", "tx9e5da8fe69be0f99",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("PDF is missing %q", want)
		}
	}
}

func TestBuildAuditReportPDFEmpty(t *testing.T) {
	out, err := BuildAuditReportPDF(models.AuditReport{Tenant: "t", Domain: "d"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
	if !bytes.Contains(out, []byte("No anchored batches")) {
		t.Error("expected the empty-state message in the PDF")
	}
}
