package anchor

import "testing"

func TestComputeHashKeyOrderIndependent(t *testing.T) {
	r1 := &Record{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"a": 1, "b": 2}}
	r2 := &Record{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"b": 2, "a": 1}}
	if r1.ComputeHash() != r2.ComputeHash() {
		t.Error("hash must be independent of payload key order")
	}
}

func TestComputeHashFields(t *testing.T) {
	r1 := &Record{ID: "1", Timestamp: "t", Source: "s", HashFields: []string{"a"}, Payload: map[string]interface{}{"a": 1, "b": 1}}
	r2 := &Record{ID: "1", Timestamp: "t", Source: "s", HashFields: []string{"a"}, Payload: map[string]interface{}{"a": 1, "b": 2}}
	if r1.ComputeHash() != r2.ComputeHash() {
		t.Error("with HashFields=[a], changing b must not change the hash")
	}
}

func TestMerkleAndLocalVerify(t *testing.T) {
	recs := []*Record{
		{ID: "1", Timestamp: "t1", Source: "s", Payload: map[string]interface{}{"a": 1}},
		{ID: "2", Timestamp: "t2", Source: "s", Payload: map[string]interface{}{"a": 2}},
		{ID: "3", Timestamp: "t3", Source: "s", Payload: map[string]interface{}{"a": 3}},
	}

	root := MerkleRoot(recs)
	if root == "" {
		t.Fatal("empty merkle root")
	}
	if !VerifyRecordsLocally(recs, root) {
		t.Error("records must verify against their own root")
	}

	// Tamper-evident: changing content breaks local verification.
	recs[1].Payload["a"] = 999
	if VerifyRecordsLocally(recs, root) {
		t.Error("tampered records must not verify against the original root")
	}
}
