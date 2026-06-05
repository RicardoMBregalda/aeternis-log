package fabric

import (
	"testing"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

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

func TestBuildChaincodeArgs(t *testing.T) {
	b := newDockerExecBackend(&config.FabricConfig{Channel: "testchannel", Chaincode: "testcc"})

	args := b.buildChaincodeArgs("testFunction", []string{"arg1", "arg2"})

	expected := `{"Args":["testFunction","arg1","arg2"]}`
	if args != expected {
		t.Errorf("Expected args %s, got %s", expected, args)
	}
}

func TestExtractTxID(t *testing.T) {
	b := newDockerExecBackend(&config.FabricConfig{})

	cases := []struct {
		name     string
		output   string
		expected string
	}{
		{"valid txid", "... result: status:200 txid:abc123def456", "abc123def456"},
		{"no txid", "Some other output", ""},
		{"multiple lines", "Line 1\nLine 2 txid:xyz789\nLine 3", "xyz789"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.extractTxID(tc.output); got != tc.expected {
				t.Errorf("Expected txid %s, got %s", tc.expected, got)
			}
		})
	}
}

// TestInvokeArgsFromConfig verifies the invoke command is built from config
// rather than hardcoded container names/paths.
func TestInvokeArgsFromConfig(t *testing.T) {
	b := newDockerExecBackend(&config.FabricConfig{
		Channel:          "ch",
		Chaincode:        "cc",
		PeerContainer:    "my-peer",
		OrdererAddress:   "my-orderer:7050",
		OrdererTLSCAFile: "/certs/ca.pem",
		TLSEnabled:       true,
	})

	args := b.invokeArgs("fn", []string{"a"})

	if len(args) < 2 || args[0] != "exec" || args[1] != "my-peer" {
		t.Fatalf("expected exec against configured peer, got %v", args)
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
	b := newDockerExecBackend(&config.FabricConfig{Channel: "ch", Chaincode: "cc", PeerContainer: "p", TLSEnabled: false})

	args := b.invokeArgs("fn", nil)
	if argsContain(args, "--tls") || argsContain(args, "--cafile") {
		t.Errorf("did not expect TLS flags when TLSEnabled is false: %v", args)
	}
}

// TestQueryArgsFromConfig verifies the query command targets the configured peer.
func TestQueryArgsFromConfig(t *testing.T) {
	b := newDockerExecBackend(&config.FabricConfig{Channel: "ch", Chaincode: "cc", PeerContainer: "qpeer"})

	args := b.queryArgs("fn", nil)
	if len(args) < 2 || args[1] != "qpeer" {
		t.Fatalf("expected exec against configured peer, got %v", args)
	}
	if !argsContain(args, "query") {
		t.Error("expected the query subcommand")
	}
}
