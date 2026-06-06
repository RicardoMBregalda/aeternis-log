//go:build integration

// Package fabric integration test for the gateway backend. Requires a running
// Fabric network with the logchaincode committed on logchannel, reachable on
// localhost:7051. Run with:
//
//	go test -tags integration ./internal/fabric/ -run TestGatewayE2E -v
package fabric

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

func TestGatewayE2E(t *testing.T) {
	const cryptoBase = "../../../hybrid-architecture/fabric-network/crypto-config"
	if _, err := os.Stat(cryptoBase); err != nil {
		t.Skipf("crypto-config not available: %v", err)
	}

	org := cryptoBase + "/peerOrganizations/org1.example.com"
	cfg := &config.FabricConfig{
		Transport:                 "gateway",
		SyncEnabled:               true,
		Channel:                   "logchannel",
		Chaincode:                 "logchaincode",
		MSPID:                     "Org1MSP",
		GatewayPeerEndpoint:       "localhost:7051",
		GatewayServerNameOverride: "peer0.org1.example.com",
		GatewayPeerTLSCAFile:      org + "/peers/peer0.org1.example.com/tls/ca.crt",
		IdentityCertFile:          org + "/users/Admin@org1.example.com/msp/signcerts/Admin@org1.example.com-cert.pem",
		IdentityKeyDir:            org + "/users/Admin@org1.example.com/msp/keystore",
		QueryTimeout:              15 * time.Second,
		InvokeTimeout:             30 * time.Second,
	}

	backend, err := newGatewayBackend(cfg)
	if err != nil {
		t.Fatalf("failed to construct gateway backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()
	batchID := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	merkleRoot := fmt.Sprintf("root-%d", time.Now().Unix())

	// Submit StoreMerkleRoot(batchID, merkleRoot, timestamp, numLogs, logIDs).
	inv, err := backend.Invoke(ctx, cfg.Channel, "StoreMerkleRoot", []string{
		batchID,
		merkleRoot,
		time.Now().UTC().Format(time.RFC3339),
		"2",
		`["log-1","log-2"]`,
	})
	if err != nil {
		t.Fatalf("Invoke StoreMerkleRoot failed: %v", err)
	}
	if inv.TxID == "" {
		t.Error("expected a transaction ID from the submit")
	}
	t.Logf("submitted batch %s, txID=%s", batchID, inv.TxID)

	// Evaluate QueryMerkleBatch(batchID) and check the round-trip.
	q, err := backend.Query(ctx, cfg.Channel, "QueryMerkleBatch", []string{batchID})
	if err != nil {
		t.Fatalf("Query QueryMerkleBatch failed: %v", err)
	}
	t.Logf("queried batch: %v", q.Data)

	if got, _ := q.Data["merkle_root"].(string); got != merkleRoot {
		t.Errorf("merkle_root mismatch: got %q, want %q (full: %v)", got, merkleRoot, q.Data)
	}
	if got, _ := q.Data["batch_id"].(string); got != batchID {
		t.Errorf("batch_id mismatch: got %q, want %q", got, batchID)
	}
}
