package fabric

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

func TestNewFabricClient(t *testing.T) {
	// Sync disabled -> the client constructs cleanly (no-op backend, no identity
	// or connectivity required).
	cfg := &config.FabricConfig{
		Channel:       "logchannel",
		Chaincode:     "logchaincode",
		Transport:     "gateway",
		SyncEnabled:   false,
		InvokeTimeout: 30 * time.Second,
		QueryTimeout:  10 * time.Second,
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
	// An unknown transport is rejected when sync is enabled.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true, Transport: "bogus"}); err == nil {
		t.Error("expected error for unknown transport")
	}
	// The legacy docker-exec transport was removed and is rejected.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true, Transport: "docker-exec"}); err == nil {
		t.Error("expected docker-exec transport to be rejected")
	}
	// Gateway (the only transport) with no identity/cert configured fails fast.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true, Transport: "gateway"}); err == nil {
		t.Error("expected error for gateway with missing identity/cert config")
	}
	// An empty transport defaults to gateway, so it too fails without identity.
	if _, err := NewFabricClient(&config.FabricConfig{SyncEnabled: true}); err == nil {
		t.Error("empty transport should default to gateway and fail without identity")
	}
}

func TestFabricClientStats(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:       "logchannel",
		Chaincode:     "logchaincode",
		Transport:     "gateway",
		SyncEnabled:   false,
		InvokeTimeout: 30 * time.Second,
		QueryTimeout:  10 * time.Second,
	}

	client, err := NewFabricClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stats := client.GetStats()

	if stats["enabled"] != false {
		t.Error("Expected enabled to be false")
	}
	if stats["transport"] != "gateway" {
		t.Errorf("Expected transport 'gateway', got %v", stats["transport"])
	}
	if stats["channel"] != "logchannel" {
		t.Errorf("Expected channel 'logchannel', got %v", stats["channel"])
	}
}

// TestFabricDisabled tests the client with sync disabled.
func TestFabricDisabled(t *testing.T) {
	client, err := NewFabricClient(&config.FabricConfig{SyncEnabled: false, Transport: "gateway"})
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
		Transport:     "gateway",
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

// Note: integration tests that actually call Fabric belong in a separate suite
// with a running Fabric network.
