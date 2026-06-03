package fabric

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

// FabricClient handles interactions with Hyperledger Fabric
type FabricClient struct {
	Config *config.FabricConfig
}

// NewFabricClient creates a new Fabric client
func NewFabricClient(cfg *config.FabricConfig) *FabricClient {
	return &FabricClient{
		Config: cfg,
	}
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

// InvokeChaincode invokes a chaincode function via docker exec
func (fc *FabricClient) InvokeChaincode(ctx context.Context, function string, args []string) (*InvokeResponse, error) {
	if !fc.Config.SyncEnabled {
		return nil, fmt.Errorf("fabric sync is disabled")
	}

	// Build peer chaincode invoke command
	cmdArgs := []string{
		"exec",
		"peer0.org1.example.com", // Container name from docker-compose
		"peer", "chaincode", "invoke",
		"-o", "orderer.example.com:7050",
		"-C", fc.Config.Channel,
		"-n", fc.Config.Chaincode,
		"--tls",
		"--cafile", "/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem",
		"-c", fc.buildChaincodeArgs(function, args),
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, fc.Config.InvokeTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", cmdArgs...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chaincode invoke failed: %w, stderr: %s", err, stderr.String())
	}

	// Parse output
	output := stdout.String()
	
	// Extract transaction ID from output
	txID := fc.extractTxID(output)
	
	response := &InvokeResponse{
		TxID:    txID,
		Status:  "success",
		Message: "Chaincode invoked successfully",
	}

	return response, nil
}

// QueryChaincode queries a chaincode function via docker exec
func (fc *FabricClient) QueryChaincode(ctx context.Context, function string, args []string) (*QueryResponse, error) {
	if !fc.Config.SyncEnabled {
		return nil, fmt.Errorf("fabric sync is disabled")
	}

	// Build peer chaincode query command
	cmdArgs := []string{
		"exec",
		"peer0.org1.example.com",
		"peer", "chaincode", "query",
		"-C", fc.Config.Channel,
		"-n", fc.Config.Chaincode,
		"-c", fc.buildChaincodeArgs(function, args),
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, fc.Config.QueryTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", cmdArgs...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chaincode query failed: %w, stderr: %s", err, stderr.String())
	}

	// Parse JSON response
	output := stdout.String()
	
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		// If not JSON, return raw output
		data = map[string]interface{}{
			"result": output,
		}
	}

	response := &QueryResponse{
		Status: "success",
		Data:   data,
	}

	return response, nil
}

// StoreMerkleBatch stores a Merkle batch in Fabric blockchain
func (fc *FabricClient) StoreMerkleBatch(ctx context.Context, batchID, merkleRoot string, numLogs int, logIDs []string) (*InvokeResponse, error) {
	// Serialize log IDs to JSON
	logIDsJSON, err := json.Marshal(logIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal log IDs: %w", err)
	}

	// Prepare arguments
	args := []string{
		batchID,
		merkleRoot,
		time.Now().UTC().Format(time.RFC3339),
		fmt.Sprintf("%d", numLogs),
		string(logIDsJSON),
	}

	// Invoke chaincode
	return fc.InvokeChaincode(ctx, "storeMerkleBatch", args)
}

// VerifyMerkleBatch verifies a Merkle batch from Fabric blockchain
func (fc *FabricClient) VerifyMerkleBatch(ctx context.Context, batchID string) (*QueryResponse, error) {
	args := []string{batchID}
	return fc.QueryChaincode(ctx, "getMerkleBatch", args)
}

// GetBatchHistory retrieves the history of a batch from Fabric
func (fc *FabricClient) GetBatchHistory(ctx context.Context, batchID string) (*QueryResponse, error) {
	args := []string{batchID}
	return fc.QueryChaincode(ctx, "getBatchHistory", args)
}

// buildChaincodeArgs builds the chaincode arguments JSON string
func (fc *FabricClient) buildChaincodeArgs(function string, args []string) string {
	argsMap := map[string]interface{}{
		"Args": append([]string{function}, args...),
	}
	
	jsonBytes, _ := json.Marshal(argsMap)
	return string(jsonBytes)
}

// extractTxID extracts the transaction ID from peer command output
func (fc *FabricClient) extractTxID(output string) string {
	// Look for pattern like "Chaincode invoke successful. result: status:200 payload:"..." txid:<txid>"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "txid:") {
			parts := strings.Split(line, "txid:")
			if len(parts) > 1 {
				txid := strings.TrimSpace(parts[1])
				// Remove any trailing characters
				txid = strings.Split(txid, " ")[0]
				return strings.Trim(txid, "\"")
			}
		}
	}
	return ""
}

// HealthCheck performs a health check on Fabric connection
func (fc *FabricClient) HealthCheck(ctx context.Context) error {
	if !fc.Config.SyncEnabled {
		return fmt.Errorf("fabric sync is disabled")
	}

	// Try to query chaincode with a simple health check function
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Check if peer container is running
	cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "name=peer0.org1.example.com", "--format", "{{.Status}}")
	
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fabric peer container not accessible: %w", err)
	}

	status := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(status, "Up") {
		return fmt.Errorf("fabric peer container is not running: %s", status)
	}

	return nil
}

// GetStats returns Fabric client statistics
func (fc *FabricClient) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":         fc.Config.SyncEnabled,
		"channel":         fc.Config.Channel,
		"chaincode":       fc.Config.Chaincode,
		"max_workers":     fc.Config.SyncMaxWorkers,
		"invoke_timeout":  fc.Config.InvokeTimeout.String(),
		"query_timeout":   fc.Config.QueryTimeout.String(),
	}
}
