package fabric

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

// FabricClient handles interactions with Hyperledger Fabric through the Fabric
// Gateway gRPC transport Backend.
type FabricClient struct {
	Config  *config.FabricConfig
	backend Backend
}

// NewFabricClient creates a new Fabric client using the configured transport.
func NewFabricClient(cfg *config.FabricConfig) (*FabricClient, error) {
	backend, err := newBackend(cfg)
	if err != nil {
		return nil, err
	}
	return &FabricClient{Config: cfg, backend: backend}, nil
}

// InvokeResponse represents the response from a chaincode invocation
type InvokeResponse struct {
	TxID    string                 `json:"tx_id"`
	Status  string                 `json:"status"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// QueryResponse represents the response from a chaincode query
type QueryResponse struct {
	Status string                 `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

// InvokeChaincode invokes a chaincode function on the given channel through the
// configured backend, using the tenant's signing identity (empty tenant uses the
// default identity).
func (fc *FabricClient) InvokeChaincode(ctx context.Context, tenant, channel, function string, args []string) (*InvokeResponse, error) {
	if !fc.Config.SyncEnabled {
		return nil, fmt.Errorf("fabric sync is disabled")
	}
	return fc.backend.Invoke(ctx, tenant, channel, function, args)
}

// QueryChaincode queries a chaincode function on the given channel through the
// configured backend, using the tenant's signing identity (empty tenant uses the
// default identity).
func (fc *FabricClient) QueryChaincode(ctx context.Context, tenant, channel, function string, args []string) (*QueryResponse, error) {
	if !fc.Config.SyncEnabled {
		return nil, fmt.Errorf("fabric sync is disabled")
	}
	return fc.backend.Query(ctx, tenant, channel, function, args)
}

// StoreMerkleBatch stores a Merkle batch for a tenant on the given channel. It
// maps to the chaincode's StoreMerkleRoot(batchID, merkleRoot, timestamp,
// numLogs, logIDs) transaction, signed with the tenant's identity so the
// chaincode partitions it under that tenant (F14). Pass the channel resolved for
// the tenant (Config.ChannelForTenant).
func (fc *FabricClient) StoreMerkleBatch(ctx context.Context, tenant, channel, batchID, merkleRoot string, numLogs int, logIDs []string) (*InvokeResponse, error) {
	logIDsJSON, err := json.Marshal(logIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal log IDs: %w", err)
	}

	args := []string{
		batchID,
		merkleRoot,
		time.Now().UTC().Format(time.RFC3339),
		fmt.Sprintf("%d", numLogs),
		string(logIDsJSON),
	}

	return fc.InvokeChaincode(ctx, tenant, channel, "StoreMerkleRoot", args)
}

// VerifyMerkleBatch reads a tenant's Merkle batch from the given channel
// (chaincode QueryMerkleBatch), signed with the tenant's identity.
func (fc *FabricClient) VerifyMerkleBatch(ctx context.Context, tenant, channel, batchID string) (*QueryResponse, error) {
	return fc.QueryChaincode(ctx, tenant, channel, "QueryMerkleBatch", []string{batchID})
}

// Enabled reports whether Fabric anchoring is turned on.
func (fc *FabricClient) Enabled() bool {
	return fc.Config.SyncEnabled
}

// ChannelForTenant resolves the Fabric channel a tenant anchors to.
func (fc *FabricClient) ChannelForTenant(tenant string) string {
	return fc.Config.ChannelForTenant(tenant)
}

// HealthCheck performs a health check on the Fabric connection.
func (fc *FabricClient) HealthCheck(ctx context.Context) error {
	if !fc.Config.SyncEnabled {
		return fmt.Errorf("fabric sync is disabled")
	}
	return fc.backend.HealthCheck(ctx)
}

// Close releases any resources held by the transport backend.
func (fc *FabricClient) Close() error {
	if fc.backend == nil {
		return nil
	}
	return fc.backend.Close()
}

// GetStats returns Fabric client statistics.
func (fc *FabricClient) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":           fc.Config.SyncEnabled,
		"transport":         fc.Config.Transport,
		"channel":           fc.Config.Channel,
		"chaincode":         fc.Config.Chaincode,
		"invoke_timeout":    fc.Config.InvokeTimeout.String(),
		"query_timeout":     fc.Config.QueryTimeout.String(),
		"tenant_identities": len(fc.Config.TenantIdentities),
	}
}
