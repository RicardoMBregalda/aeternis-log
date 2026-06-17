package models

import "testing"

func recForHash(id, ts, source string) *Record {
	return &Record{ID: id, Timestamp: ts, Source: source, Payload: map[string]interface{}{}}
}

// TestLeafHashV2NoBoundaryCollision: under v1 (plain concat) two records that
// shift a character across the ID/Timestamp boundary collide; v2 must not.
func TestLeafHashV2NoBoundaryCollision(t *testing.T) {
	a := recForHash("ab", "", "s")
	b := recForHash("a", "b", "s")

	if a.calculateHashV1() != b.calculateHashV1() {
		t.Fatal("premise: v1 should collide on a field-boundary shift")
	}
	if a.calculateHashV2() == b.calculateHashV2() {
		t.Error("v2 leaf hash must NOT collide across field boundaries")
	}
}

// TestMerkleV2DomainSeparation: an internal node must be domain-separated from a
// leaf, so the v2 combine must differ from the v1 (undifferentiated) combine.
func TestMerkleV2DomainSeparation(t *testing.T) {
	if CombineHashesV2("x", "y") == CombineHashes("x", "y") {
		t.Error("v2 internal node must be domain-separated (differ from the v1 combine)")
	}
}

// TestMerkleV2OddCountNotDuplicated: the CVE-2012-2459 fix. Under v1, a 3-leaf
// tree duplicates the last leaf, making it equal to the 4-leaf tree [a,b,c,c].
// v2 promotes the odd node instead, so the two roots must differ.
func TestMerkleV2OddCountNotDuplicated(t *testing.T) {
	three := []string{"a", "b", "c"}
	fourDup := []string{"a", "b", "c", "c"}

	if BuildMerkleTree(three) != BuildMerkleTree(fourDup) {
		t.Fatal("premise: v1 is expected to duplicate the last node (CVE-2012-2459)")
	}
	if BuildMerkleTreeV2(three) == BuildMerkleTreeV2(fourDup) {
		t.Error("v2 must not duplicate the last node: 3-leaf and [a,b,c,c] roots must differ")
	}
	// Determinism + structure sensitivity.
	if got := BuildMerkleTreeV2(three); got == "" || got != BuildMerkleTreeV2(three) {
		t.Error("v2 root must be deterministic and non-empty")
	}
	if BuildMerkleTreeV2([]string{"a", "b"}) == BuildMerkleTreeV2([]string{"a", "z"}) {
		t.Error("different leaves must yield different roots")
	}
}

// TestRecordMerkleRootVersionDispatch: a v1 batch verifies under v1, a v2 batch
// under v2, and the two roots differ for the same content (so old anchors stay
// valid while new ones use the hardened scheme).
func TestRecordMerkleRootVersionDispatch(t *testing.T) {
	mk := func(v int) []*Record {
		return []*Record{
			{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"n": 1}, HashVersion: v},
			{ID: "2", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"n": 2}, HashVersion: v},
			{ID: "3", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"n": 3}, HashVersion: v},
		}
	}
	rootV1, _ := CalculateRecordMerkleRoot(mk(1))
	rootV2, _ := CalculateRecordMerkleRoot(mk(2))

	if rootV1 == "" || rootV2 == "" {
		t.Fatal("roots must be non-empty")
	}
	if rootV1 == rootV2 {
		t.Error("v1 and v2 roots must differ for the same content (scheme is versioned)")
	}
}

// TestConformanceVectorV2 pins the v2 scheme to a golden vector shared with the
// Go and Python SDK tests, so all three implementations agree byte-for-byte.
func TestConformanceVectorV2(t *testing.T) {
	const wantRoot = "897b88ee0e8ce4bc54391fd8de95d716c139f0ec972409d7024d64a1835eaf2b"
	const wantLeaf1 = "d45b3f3276931bfde83931f183e470620d0289f7d6ef3de77b80a8465940c13c"
	recs := []*Record{
		{ID: "rec-1", Timestamp: "2026-01-01T00:00:00Z", Source: "conformance", Payload: map[string]interface{}{"k": "a"}, HashVersion: 2},
		{ID: "rec-2", Timestamp: "2026-01-01T00:00:00Z", Source: "conformance", Payload: map[string]interface{}{"k": "b"}, HashVersion: 2},
		{ID: "rec-3", Timestamp: "2026-01-01T00:00:00Z", Source: "conformance", Payload: map[string]interface{}{"k": "c"}, HashVersion: 2},
	}
	if got := recs[0].CalculateHash(); got != wantLeaf1 {
		t.Errorf("leaf1 = %s, want %s", got, wantLeaf1)
	}
	if root, _ := CalculateRecordMerkleRoot(recs); root != wantRoot {
		t.Errorf("root = %s, want %s", root, wantRoot)
	}
}
