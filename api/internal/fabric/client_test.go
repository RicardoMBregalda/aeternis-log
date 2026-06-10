package fabric

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

func TestNewFabricClient(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:        "logchannel",
		Chaincode:      "logchaincode",
		Transport:      "docker-exec",
		SyncEnabled:    true,
		SyncMaxWorkers: 10,
		InvokeTimeout:  30 * time.Second,
		QueryTimeout:   10 * time.Second,
	}

	client, err := NewFabricClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("Expected client to be created")
	}
	if client.Config.Channel != "logchannel" {
		t.Errorf("Expected channel 'logchannel', got %s", client.Config.Channel)
	}
}

func TestNewFabricClientTransport(t *testing.T) {
	// Disabled sync constructs cleanly regardless of transport (no backend).
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: false, Transport: "gateway"}); err != nil {
		t.Errorf("disabled sync should not construct a backend: %v", err)
	}
	// Unknown transport is rejected when sync is enabled.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true, Transport: "bogus"}); err == nil {
		t.Error("expected error for unknown transport")
	}
	// Gateway with no identity/cert configured fails to construct.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true, Transport: "gateway"}); err == nil {
		t.Error("expected error for gateway with missing identity/cert config")
	}
	// Empty transport with sync enabled defaults to docker-exec.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true}); err != nil {
		t.Errorf("empty transport should default to docker-exec: %v", err)
	}
}

func TestFabricClientStats(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:        "logchannel",
		Chaincode:      "logchaincode",
		Transport:      "docker-exec",
		PeerContainer:  "peer0",
		SyncEnabled:    true,
		SyncMaxWorkers: 10,
		InvokeTimeout:  30 * time.Second,
		QueryTimeout:   10 * time.Second,
	}

	client, err := NewFabricClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stats := client.GetStats()

	if stats["enabled"] != true {
		t.Error("Expected enabled to be true")
	}
	if stats["transport"] != "docker-exec" {
		t.Errorf("Expected transport 'docker-exec', got %v", stats["transport"])
	}
	if stats["channel"] != "logchannel" {
		t.Errorf("Expected channel 'logchannel', got %v", stats["channel"])
	}
	if stats["max_workers"] != 10 {
		t.Errorf("Expected max_workers 10, got %v", stats["max_workers"])
	}
}

// TestFabricDisabled tests client with sync disabled.
func TestFabricDisabled(t *testing.T) {
	client, err := NewFabricClient(&config.FabricConfig{SyncEnabled: false, Transport: "docker-exec"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()

	if _, err := client.InvokeChaincode(ctx, "logchannel", "test", []string{}); err == nil {
		t.Error("Expected error when sync is disabled")
	}
	if _, err := client.QueryChaincode(ctx, "logchannel", "test", []string{}); err == nil {
		t.Error("Expected error when sync is disabled")
	}
}

// TestStoreMerkleBatchDisabled checks the high-level method fails fast when sync
// is disabled (without touching the backend).
func TestStoreMerkleBatchDisabled(t *testing.T) {
	client, err := NewFabricClient(&config.FabricConfig{
		Channel:       "logchannel",
		Chaincode:     "logchaincode",
		Transport:     "docker-exec",
		SyncEnabled:   false,
		InvokeTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := client.StoreMerkleBatch(context.Background(), "logchannel", "batch_123", "merkle_abc", 3, []string{"l1", "l2"}); err == nil {
		t.Error("Expected error when sync is disabled")
	}
}

// Note: integration tests that actually call docker/Fabric belong in a separate
// suite with a running Fabric network.
