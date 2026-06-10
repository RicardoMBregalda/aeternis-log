package handlers

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	id := "abc-123"

	enc := encodeCursor(now, id)
	if enc == "" {
		t.Fatal("encoded cursor is empty")
	}

	got, err := decodeCursor(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != id {
		t.Errorf("id: got %q, want %q", got.ID, id)
	}
	// created_at round-trips at millisecond precision (matching storage).
	if got.TimeMillis != now.UnixMilli() {
		t.Errorf("time: got %d, want %d", got.TimeMillis, now.UnixMilli())
	}
}

func TestDecodeCursorInvalid(t *testing.T) {
	if _, err := decodeCursor("!!! not base64 !!!"); err == nil {
		t.Error("expected error for invalid base64")
	}
	// "abc" is valid base64 but not valid JSON.
	if _, err := decodeCursor("YWJj"); err == nil {
		t.Error("expected error for non-JSON payload")
	}
}
