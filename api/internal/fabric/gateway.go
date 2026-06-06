package fabric

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

// gatewayBackend talks to the peer through the Fabric Gateway gRPC API. It owns
// a persistent gRPC connection and resolves the chaincode contract per channel
// (one connection serves every channel the peer has joined), removing the need
// for the Docker socket and the peer CLI. Contracts are cached per channel so
// per-tenant channels reuse the same gateway.
type gatewayBackend struct {
	conn      *grpc.ClientConn
	gateway   *client.Gateway
	chaincode string

	mu        sync.Mutex
	contracts map[string]*client.Contract
}

// contractFor returns the chaincode contract on the given channel, caching it.
func (b *gatewayBackend) contractFor(channel string) *client.Contract {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.contracts[channel]; ok {
		return c
	}
	c := b.gateway.GetNetwork(channel).GetContract(b.chaincode)
	b.contracts[channel] = c
	return c
}

// newGatewayBackend builds a gateway-backed Fabric transport: it loads the
// signing identity and peer TLS root from disk and opens the gRPC connection.
func newGatewayBackend(cfg *config.FabricConfig) (*gatewayBackend, error) {
	conn, err := newGRPCConnection(cfg)
	if err != nil {
		return nil, err
	}

	id, err := newIdentity(cfg.IdentityCertFile, cfg.MSPID)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	sign, err := newSign(cfg.IdentityKeyDir)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	gw, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(cfg.QueryTimeout),
		client.WithEndorseTimeout(cfg.InvokeTimeout),
		client.WithSubmitTimeout(cfg.InvokeTimeout),
		client.WithCommitStatusTimeout(cfg.InvokeTimeout),
	)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to connect Fabric gateway: %w", err)
	}

	return &gatewayBackend{
		conn:      conn,
		gateway:   gw,
		chaincode: cfg.Chaincode,
		contracts: make(map[string]*client.Contract),
	}, nil
}

// newGRPCConnection dials the peer gateway endpoint using its TLS root cert.
func newGRPCConnection(cfg *config.FabricConfig) (*grpc.ClientConn, error) {
	pem, err := os.ReadFile(cfg.GatewayPeerTLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read peer TLS cert: %w", err)
	}
	cert, err := identity.CertificateFromPEM(pem)
	if err != nil {
		return nil, fmt.Errorf("failed to parse peer TLS cert: %w", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	creds := credentials.NewClientTLSFromCert(pool, cfg.GatewayServerNameOverride)

	conn, err := grpc.NewClient(cfg.GatewayPeerEndpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for peer %s: %w", cfg.GatewayPeerEndpoint, err)
	}
	return conn, nil
}

// newIdentity builds an X.509 identity from the signing certificate.
func newIdentity(certFile, mspID string) (*identity.X509Identity, error) {
	pem, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity cert: %w", err)
	}
	cert, err := identity.CertificateFromPEM(pem)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identity cert: %w", err)
	}
	id, err := identity.NewX509Identity(mspID, cert)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity: %w", err)
	}
	return id, nil
}

// newSign loads the (single) private key from the MSP keystore directory.
func newSign(keyDir string) (identity.Sign, error) {
	entries, err := os.ReadDir(keyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read key directory: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no private key found in %s", keyDir)
	}
	pem, err := os.ReadFile(filepath.Join(keyDir, entries[0].Name()))
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	key, err := identity.PrivateKeyFromPEM(pem)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	sign, err := identity.NewPrivateKeySign(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}
	return sign, nil
}

// Invoke submits a transaction (endorse + submit) on the given channel and
// returns its transaction ID.
func (b *gatewayBackend) Invoke(_ context.Context, channel, function string, args []string) (*InvokeResponse, error) {
	proposal, err := b.contractFor(channel).NewProposal(function, client.WithArguments(args...))
	if err != nil {
		return nil, fmt.Errorf("failed to create proposal: %w", err)
	}
	transaction, err := proposal.Endorse()
	if err != nil {
		return nil, fmt.Errorf("failed to endorse transaction: %w", err)
	}
	result := transaction.Result()

	commit, err := transaction.Submit()
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}
	if status, serr := commit.Status(); serr == nil && !status.Successful {
		return nil, fmt.Errorf("transaction %s failed to commit (validation code %v)", status.TransactionID, status.Code)
	}

	return &InvokeResponse{
		TxID:    transaction.TransactionID(),
		Status:  "success",
		Message: "Chaincode invoked successfully",
		Data:    parseJSONResult(result),
	}, nil
}

// Query evaluates a transaction (no ledger update) on the given channel and
// returns the result.
func (b *gatewayBackend) Query(_ context.Context, channel, function string, args []string) (*QueryResponse, error) {
	result, err := b.contractFor(channel).EvaluateTransaction(function, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate transaction: %w", err)
	}
	return &QueryResponse{Status: "success", Data: parseJSONResult(result)}, nil
}

// HealthCheck reports whether the gRPC connection is usable.
func (b *gatewayBackend) HealthCheck(_ context.Context) error {
	if b.conn.GetState() == connectivity.Shutdown {
		return fmt.Errorf("fabric gateway connection is shut down")
	}
	return nil
}

// Close releases the gateway and the underlying gRPC connection.
func (b *gatewayBackend) Close() error {
	if b.gateway != nil {
		b.gateway.Close()
	}
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

// parseJSONResult decodes a chaincode result into a map, falling back to a raw
// string under "result" when it is not a JSON object.
func parseJSONResult(b []byte) map[string]interface{} {
	if len(b) == 0 {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return map[string]interface{}{"result": string(b)}
	}
	return data
}
