package fabric

import (
	"os"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

// TestGatewayBackendConstruct loads the real Admin@org1 identity and peer TLS
// root from the repo's crypto-config and verifies the gateway backend builds.
// grpc.Dial is lazy, so this validates PEM/identity handling without requiring
// a running Fabric network. It skips when the crypto material is absent.
func TestGatewayBackendConstruct(t *testing.T) {
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
		GatewayPeerEndpoint:       "peer0.org1.example.com:7051",
		GatewayServerNameOverride: "peer0.org1.example.com",
		GatewayPeerTLSCAFile:      org + "/peers/peer0.org1.example.com/tls/ca.crt",
		IdentityCertFile:          org + "/users/Admin@org1.example.com/msp/signcerts/Admin@org1.example.com-cert.pem",
		IdentityKeyDir:            org + "/users/Admin@org1.example.com/msp/keystore",
		QueryTimeout:              5 * time.Second,
		InvokeTimeout:             15 * time.Second,
	}

	b, err := newGatewayBackend(cfg)
	if err != nil {
		t.Fatalf("failed to construct gateway backend with real crypto material: %v", err)
	}
	defer b.Close()

	if b.contract == nil {
		t.Error("expected a non-nil contract handle")
	}
	if err := b.HealthCheck(nil); err != nil {
		t.Errorf("health check on a fresh connection should pass: %v", err)
	}
}
