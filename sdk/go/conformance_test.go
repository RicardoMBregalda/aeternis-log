package aeternislog

import "testing"

// Golden vector produced by the server's authoritative v2 scheme. The Go SDK,
// the Python SDK and the server must all reproduce it byte-for-byte, so a
// locally recomputed root can be trusted against the on-chain anchor.
const (
	conformanceV2Root  = "897b88ee0e8ce4bc54391fd8de95d716c139f0ec972409d7024d64a1835eaf2b"
	conformanceV2Leaf1 = "d45b3f3276931bfde83931f183e470620d0289f7d6ef3de77b80a8465940c13c"
)

func conformanceRecords() []*Record {
	mk := func(id, k string) *Record {
		return &Record{
			ID: id, Timestamp: "2026-01-01T00:00:00Z", Source: "conformance",
			Payload: map[string]interface{}{"k": k}, HashVersion: 2,
		}
	}
	return []*Record{mk("rec-1", "a"), mk("rec-2", "b"), mk("rec-3", "c")}
}

func TestConformanceV2(t *testing.T) {
	recs := conformanceRecords()
	if got := recs[0].ComputeHash(); got != conformanceV2Leaf1 {
		t.Errorf("leaf hash = %s, want %s", got, conformanceV2Leaf1)
	}
	if got := MerkleRoot(recs); got != conformanceV2Root {
		t.Errorf("merkle root = %s, want %s", got, conformanceV2Root)
	}
	if !VerifyRecordsLocally(recs, conformanceV2Root) {
		t.Error("VerifyRecordsLocally should pass for the conformance vector")
	}
}
