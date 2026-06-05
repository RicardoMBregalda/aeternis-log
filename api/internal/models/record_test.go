package models

import "testing"

// TestRecordHashKeyOrderIndependent is the core integrity property: the hash
// must not depend on payload key insertion order (otherwise verification of a
// re-serialized record would falsely fail).
func TestRecordHashKeyOrderIndependent(t *testing.T) {
	r1 := &Record{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"a": 1, "b": 2, "c": 3}}
	r2 := &Record{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"c": 3, "a": 1, "b": 2}}

	if r1.CalculateHash() != r2.CalculateHash() {
		t.Error("hash must be independent of payload key order")
	}
}

func TestRecordHashChangesWithContent(t *testing.T) {
	base := &Record{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"a": 1}}
	changed := &Record{ID: "1", Timestamp: "t", Source: "s", Payload: map[string]interface{}{"a": 2}}

	if base.CalculateHash() == changed.CalculateHash() {
		t.Error("hash must change when the payload changes")
	}
}

// TestRecordHashFields verifies the configurable hash: only the selected payload
// fields feed the hash, so changes outside them do not affect it.
func TestRecordHashFields(t *testing.T) {
	r1 := &Record{ID: "1", Timestamp: "t", Source: "s", HashFields: []string{"a"}, Payload: map[string]interface{}{"a": 1, "b": 1}}
	rOtherB := &Record{ID: "1", Timestamp: "t", Source: "s", HashFields: []string{"a"}, Payload: map[string]interface{}{"a": 1, "b": 999}}
	rOtherA := &Record{ID: "1", Timestamp: "t", Source: "s", HashFields: []string{"a"}, Payload: map[string]interface{}{"a": 2, "b": 1}}

	if r1.CalculateHash() != rOtherB.CalculateHash() {
		t.Error("with HashFields=[a], changing b must not change the hash")
	}
	if r1.CalculateHash() == rOtherA.CalculateHash() {
		t.Error("with HashFields=[a], changing a must change the hash")
	}
}

func TestRecordValidate(t *testing.T) {
	valid := &Record{Domain: "d", ID: "1", Source: "s", Payload: map[string]interface{}{}}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid record: unexpected error: %v", err)
	}

	invalid := map[string]*Record{
		"no domain":  {ID: "1", Source: "s", Payload: map[string]interface{}{}},
		"no id":      {Domain: "d", Source: "s", Payload: map[string]interface{}{}},
		"no source":  {Domain: "d", ID: "1", Payload: map[string]interface{}{}},
		"no payload": {Domain: "d", ID: "1", Source: "s"},
	}
	for name, r := range invalid {
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}
