//go:build integration

// Integration test for the SDK against a running API. Run with:
//
//	go test -tags integration ./... -run TestSDKLive -v
//
// Override the base URL with AETERNISLOG_BASE_URL (default http://localhost:5001).
package aeternislog

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSDKLive(t *testing.T) {
	base := os.Getenv("AETERNISLOG_BASE_URL")
	if base == "" {
		base = "http://localhost:5001"
	}
	c := New(base, WithAPIKey(os.Getenv("AETERNISLOG_API_KEY")))
	ctx := context.Background()
	const domain = "sdktest"

	// Create a record; CreateRecord checks the server hash == the local hash.
	rec, err := c.CreateRecord(ctx, domain, "sdk", map[string]interface{}{"foo": "bar", "n": 7}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("created %s hash=%s", rec.ID, rec.Hash)

	// Fetch it back; its independently recomputed hash must match the stored one.
	got, err := c.GetRecord(ctx, domain, rec.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ComputeHash() != got.Hash {
		t.Errorf("local hash %s != stored hash %s", got.ComputeHash(), got.Hash)
	}

	// Batch + anchor, then verify server-side.
	time.Sleep(150 * time.Millisecond)
	res, err := c.BatchRecords(ctx, domain)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if !res.Anchored || res.TxID == "" {
		t.Fatalf("expected an anchored batch with a tx id, got %+v", res)
	}
	t.Logf("batch %s anchored root=%s tx=%s", res.BatchID, res.MerkleRoot, res.TxID)

	v, err := c.VerifyBatch(ctx, domain, res.BatchID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !v.IsValid {
		t.Errorf("expected VALID, got %+v", v)
	}
}
