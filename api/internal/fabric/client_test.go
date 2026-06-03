package fabric

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

// TestNewFabricClient tests Fabric client creation
func TestNewFabricClient(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:        "logchannel",
		Chaincode:      "logchaincode",
		SyncEnabled:    true,
		SyncMaxWorkers: 10,
		InvokeTimeout:  30 * time.Second,
		QueryTimeout:   10 * time.Second,
	}

	client := NewFabricClient(cfg)

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.Config.Channel != "logchannel" {
		t.Errorf("Expected channel 'logchannel', got %s", client.Config.Channel)
	}
}

// TestBuildChaincodeArgs tests chaincode argument building
func TestBuildChaincodeArgs(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:   "testchannel",
		Chaincode: "testcc",
	}

	client := NewFabricClient(cfg)

	args := client.buildChaincodeArgs("testFunction", []string{"arg1", "arg2"})

	if args == "" {
		t.Error("Expected non-empty args string")
	}

	expected := `{"Args":["testFunction","arg1","arg2"]}`
	if args != expected {
		t.Errorf("Expected args %s, got %s", expected, args)
	}
}

// TestExtractTxID tests transaction ID extraction
func TestExtractTxID(t *testing.T) {
	cfg := &config.FabricConfig{}
	client := NewFabricClient(cfg)

	testCases := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "Valid txid",
			output:   "2024-01-01 12:00:00.000 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200 txid:abc123def456",
			expected: "abc123def456",
		},
		{
			name:     "No txid",
			output:   "Some other output",
			expected: "",
		},
		{
			name:     "Multiple lines with txid",
			output:   "Line 1\nLine 2 txid:xyz789\nLine 3",
			expected: "xyz789",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.extractTxID(tc.output)
			if result != tc.expected {
				t.Errorf("Expected txid %s, got %s", tc.expected, result)
			}
		})
	}
}

// TestFabricClientStats tests stats retrieval
func TestFabricClientStats(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:        "logchannel",
		Chaincode:      "logchaincode",
		SyncEnabled:    true,
		SyncMaxWorkers: 10,
		InvokeTimeout:  30 * time.Second,
		QueryTimeout:   10 * time.Second,
	}

	client := NewFabricClient(cfg)
	stats := client.GetStats()

	if stats["enabled"] != true {
		t.Error("Expected enabled to be true")
	}

	if stats["channel"] != "logchannel" {
		t.Errorf("Expected channel 'logchannel', got %v", stats["channel"])
	}

	if stats["max_workers"] != 10 {
		t.Errorf("Expected max_workers 10, got %v", stats["max_workers"])
	}
}

// TestFabricDisabled tests client with sync disabled
func TestFabricDisabled(t *testing.T) {
	cfg := &config.FabricConfig{
		SyncEnabled: false,
	}

	client := NewFabricClient(cfg)
	ctx := context.Background()

	// Should return error when sync is disabled
	_, err := client.InvokeChaincode(ctx, "test", []string{})
	if err == nil {
		t.Error("Expected error when sync is disabled")
	}

	_, err = client.QueryChaincode(ctx, "test", []string{})
	if err == nil {
		t.Error("Expected error when sync is disabled")
	}
}

// TestStoreMerkleBatch tests Merkle batch storage
func TestStoreMerkleBatch(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:       "logchannel",
		Chaincode:     "logchaincode",
		SyncEnabled:   false, // Disabled for unit test
		InvokeTimeout: 30 * time.Second,
	}

	client := NewFabricClient(cfg)
	ctx := context.Background()

	logIDs := []string{"log1", "log2", "log3"}

	// Should fail because sync is disabled
	_, err := client.StoreMerkleBatch(ctx, "batch_123", "merkle_abc", 3, logIDs)
	if err == nil {
		t.Error("Expected error when sync is disabled")
	}
}

// Note: Integration tests that actually call docker/Fabric should be in
// a separate integration test suite with proper Fabric network setup
