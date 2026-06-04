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

func argValue(args []string, flag string) (string, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1], true
		}
	}
	return "", false
}

func argsContain(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

// TestInvokeArgsFromConfig verifies the invoke command is built from config
// rather than hardcoded container names/paths.
func TestInvokeArgsFromConfig(t *testing.T) {
	cfg := &config.FabricConfig{
		Channel:          "ch",
		Chaincode:        "cc",
		PeerContainer:    "my-peer",
		OrdererAddress:   "my-orderer:7050",
		OrdererTLSCAFile: "/certs/ca.pem",
		TLSEnabled:       true,
	}
	args := NewFabricClient(cfg).invokeArgs("fn", []string{"a"})

	if len(args) < 2 || args[0] != "exec" || args[1] != "my-peer" {
		t.Fatalf("expected exec against configured peer, got %v", args[:min(2, len(args))])
	}
	if v, ok := argValue(args, "-o"); !ok || v != "my-orderer:7050" {
		t.Errorf("orderer address: got %q (ok=%v)", v, ok)
	}
	if v, ok := argValue(args, "-C"); !ok || v != "ch" {
		t.Errorf("channel: got %q", v)
	}
	if v, ok := argValue(args, "--cafile"); !ok || v != "/certs/ca.pem" {
		t.Errorf("cafile: got %q", v)
	}
	if !argsContain(args, "--tls") {
		t.Error("expected --tls when TLSEnabled is true")
	}
}

// TestInvokeArgsNoTLS verifies TLS flags are omitted when TLS is disabled.
func TestInvokeArgsNoTLS(t *testing.T) {
	cfg := &config.FabricConfig{Channel: "ch", Chaincode: "cc", PeerContainer: "p", TLSEnabled: false}
	args := NewFabricClient(cfg).invokeArgs("fn", nil)
	if argsContain(args, "--tls") || argsContain(args, "--cafile") {
		t.Errorf("did not expect TLS flags when TLSEnabled is false: %v", args)
	}
}

// TestQueryArgsFromConfig verifies the query command targets the configured peer.
func TestQueryArgsFromConfig(t *testing.T) {
	cfg := &config.FabricConfig{Channel: "ch", Chaincode: "cc", PeerContainer: "qpeer"}
	args := NewFabricClient(cfg).queryArgs("fn", nil)
	if len(args) < 2 || args[1] != "qpeer" {
		t.Fatalf("expected exec against configured peer, got %v", args)
	}
	if !argsContain(args, "query") {
		t.Error("expected the query subcommand")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Note: Integration tests that actually call docker/Fabric should be in
// a separate integration test suite with proper Fabric network setup
