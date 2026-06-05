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

// dockerExecBackend talks to the peer by shelling out to
// `docker exec <peer> peer chaincode ...`. It requires the Docker socket and the
// peer CLI inside the peer container. This is the legacy transport, kept behind
// the Backend interface and superseded by the gateway (gRPC) backend.
type dockerExecBackend struct {
	cfg *config.FabricConfig
}

func newDockerExecBackend(cfg *config.FabricConfig) *dockerExecBackend {
	return &dockerExecBackend{cfg: cfg}
}

// Invoke runs a chaincode invoke and extracts the transaction ID from the CLI output.
func (b *dockerExecBackend) Invoke(ctx context.Context, function string, args []string) (*InvokeResponse, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, b.cfg.InvokeTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", b.invokeArgs(function, args)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chaincode invoke failed: %w, stderr: %s", err, stderr.String())
	}

	return &InvokeResponse{
		TxID:    b.extractTxID(stdout.String()),
		Status:  "success",
		Message: "Chaincode invoked successfully",
	}, nil
}

// Query runs a chaincode query and decodes the JSON result.
func (b *dockerExecBackend) Query(ctx context.Context, function string, args []string) (*QueryResponse, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, b.cfg.QueryTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", b.queryArgs(function, args)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("chaincode query failed: %w, stderr: %s", err, stderr.String())
	}

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		// Not JSON: return the raw output.
		data = map[string]interface{}{"result": stdout.String()}
	}

	return &QueryResponse{Status: "success", Data: data}, nil
}

// HealthCheck verifies the peer container is up via `docker ps`.
func (b *dockerExecBackend) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "name="+b.cfg.PeerContainer, "--format", "{{.Status}}")
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

// Close is a no-op for the docker-exec backend (no persistent connection).
func (b *dockerExecBackend) Close() error { return nil }

// invokeArgs builds the `docker exec ... peer chaincode invoke` argument list
// from configuration, so peer/orderer/cert locations are not hardcoded.
func (b *dockerExecBackend) invokeArgs(function string, args []string) []string {
	cmdArgs := []string{
		"exec",
		b.cfg.PeerContainer,
		"peer", "chaincode", "invoke",
		"-o", b.cfg.OrdererAddress,
		"-C", b.cfg.Channel,
		"-n", b.cfg.Chaincode,
	}
	if b.cfg.TLSEnabled {
		cmdArgs = append(cmdArgs, "--tls", "--cafile", b.cfg.OrdererTLSCAFile)
	}
	return append(cmdArgs, "-c", b.buildChaincodeArgs(function, args))
}

// queryArgs builds the `docker exec ... peer chaincode query` argument list.
func (b *dockerExecBackend) queryArgs(function string, args []string) []string {
	return []string{
		"exec",
		b.cfg.PeerContainer,
		"peer", "chaincode", "query",
		"-C", b.cfg.Channel,
		"-n", b.cfg.Chaincode,
		"-c", b.buildChaincodeArgs(function, args),
	}
}

// buildChaincodeArgs builds the chaincode arguments JSON string.
func (b *dockerExecBackend) buildChaincodeArgs(function string, args []string) string {
	argsMap := map[string]interface{}{
		"Args": append([]string{function}, args...),
	}
	jsonBytes, _ := json.Marshal(argsMap)
	return string(jsonBytes)
}

// extractTxID extracts the transaction ID from peer command output.
func (b *dockerExecBackend) extractTxID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "txid:") {
			parts := strings.Split(line, "txid:")
			if len(parts) > 1 {
				txid := strings.TrimSpace(parts[1])
				txid = strings.Split(txid, " ")[0]
				return strings.Trim(txid, "\"")
			}
		}
	}
	return ""
}
