package fabric

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

// gatewayConn is one signing identity's view of the peer: its own gateway
// session plus a per-channel contract cache.
type gatewayConn struct {
	gateway   *client.Gateway
	chaincode string

	mu        sync.Mutex
	contracts map[string]*client.Contract
}

// contractFor returns the chaincode contract on the given channel, caching it.
func (g *gatewayConn) contractFor(channel string) *client.Contract {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.contracts[channel]; ok {
		return c
	}
	c := g.gateway.GetNetwork(channel).GetContract(g.chaincode)
	g.contracts[channel] = c
	return c
}

func (g *gatewayConn) close() {
	if g != nil && g.gateway != nil {
		g.gateway.Close()
	}
}

// gatewayBackend talks to the peer through the Fabric Gateway gRPC API. It owns
// a single persistent gRPC connection (one connection serves every channel the
// peer has joined) shared by one gateway session per signing identity: a default
// identity plus an optional per-tenant identity (F14). Selecting the identity by
// tenant is how a tenant anchors under a certificate carrying its own `tenant`
// attribute; an unmapped tenant uses the default identity, so the change is
// backward-compatible when no per-tenant identities are configured.
type gatewayBackend struct {
	conn      *grpc.ClientConn
	defaultGw *gatewayConn
	tenantGws map[string]*gatewayConn
}

// gatewayFor returns the gateway session for a tenant: its own when configured,
// otherwise the default identity.
func (b *gatewayBackend) gatewayFor(tenant string) *gatewayConn {
	if g, ok := b.tenantGws[tenant]; ok {
		return g
	}
	return b.defaultGw
}

// newGatewayBackend builds a gateway-backed Fabric transport: a single gRPC
// connection shared by the default signing identity and any per-tenant
// identities configured under fabric.tenant_identities.
func newGatewayBackend(cfg *config.FabricConfig) (*gatewayBackend, error) {
	conn, err := newGRPCConnection(cfg)
	if err != nil {
		return nil, err
	}

	b := &gatewayBackend{conn: conn, tenantGws: make(map[string]*gatewayConn)}

	b.defaultGw, err = connectGateway(conn, cfg, cfg.MSPID, cfg.IdentityCertFile, cfg.IdentityKeyDir)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	for tenant, ti := range cfg.TenantIdentities {
		mspID := ti.MSPID
		if mspID == "" {
			mspID = cfg.MSPID
		}
		gw, err := connectGateway(conn, cfg, mspID, ti.IdentityCertFile, ti.IdentityKeyDir)
		if err != nil {
			b.Close()
			return nil, fmt.Errorf("tenant %q identity: %w", tenant, err)
		}
		b.tenantGws[tenant] = gw
	}

	return b, nil
}

// connectGateway opens a gateway session for one signing identity over the given
// shared gRPC connection.
func connectGateway(conn *grpc.ClientConn, cfg *config.FabricConfig, mspID, certFile, keyDir string) (*gatewayConn, error) {
	id, err := newIdentity(certFile, mspID)
	if err != nil {
		return nil, err
	}
	sign, err := newSign(keyDir)
	if err != nil {
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
		return nil, fmt.Errorf("failed to connect Fabric gateway: %w", err)
	}
	return &gatewayConn{gateway: gw, chaincode: cfg.Chaincode, contracts: make(map[string]*client.Contract)}, nil
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

// Invoke submits a transaction (endorse + submit) on the given channel using the
// tenant's signing identity (or the default identity when unmapped) and returns
// its transaction ID.
func (b *gatewayBackend) Invoke(ctx context.Context, tenant, channel, function string, args []string) (*InvokeResponse, error) {
	proposal, err := b.gatewayFor(tenant).contractFor(channel).NewProposal(function, client.WithArguments(args...))
	if err != nil {
		return nil, fmt.Errorf("failed to create proposal: %w", err)
	}
	transaction, err := proposal.EndorseWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to endorse transaction: %w", err)
	}
	result := transaction.Result()

	commit, err := transaction.SubmitWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}
	// A commit-status error (timeout, dropped connection) means the outcome is
	// UNKNOWN — it must not be reported as a successful anchor. Treat it as a
	// failure so the caller (and the reconciler) can re-drive the batch.
	status, serr := commit.StatusWithContext(ctx)
	if serr != nil {
		return nil, fmt.Errorf("failed to obtain commit status for transaction %s: %w", transaction.TransactionID(), serr)
	}
	if !status.Successful {
		return nil, fmt.Errorf("transaction %s failed to commit (validation code %v)", status.TransactionID, status.Code)
	}

	return &InvokeResponse{
		TxID:    transaction.TransactionID(),
		Status:  "success",
		Message: "Chaincode invoked successfully",
		Data:    parseJSONResult(result),
	}, nil
}

// Query evaluates a transaction (no ledger update) on the given channel using
// the tenant's signing identity (or the default when unmapped).
func (b *gatewayBackend) Query(ctx context.Context, tenant, channel, function string, args []string) (*QueryResponse, error) {
	result, err := b.gatewayFor(tenant).contractFor(channel).EvaluateWithContext(ctx, function, client.WithArguments(args...))
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

// Close releases every gateway session and the underlying gRPC connection.
func (b *gatewayBackend) Close() error {
	b.defaultGw.close()
	for _, g := range b.tenantGws {
		g.close()
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
