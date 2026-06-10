package fabric

import (
	"context"
	"fmt"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

// Backend is the transport used to talk to the Fabric peer. The docker-exec
// backend shells out to the peer CLI; the gateway backend uses the Fabric
// Gateway gRPC SDK. Selecting the backend is a configuration concern, so the
// rest of the API depends only on this interface.
type Backend interface {
	Invoke(ctx context.Context, channel, function string, args []string) (*InvokeResponse, error)
	Query(ctx context.Context, channel, function string, args []string) (*QueryResponse, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

var (
	_ Backend = (*dockerExecBackend)(nil)
	_ Backend = (*gatewayBackend)(nil)
	_ Backend = disabledBackend{}
)

// newBackend selects the Fabric transport backend from configuration. When sync
// is disabled it returns a no-op backend so the client constructs cleanly
// without requiring connectivity or credentials.
func newBackend(cfg *config.FabricConfig) (Backend, error) {
	if !cfg.SyncEnabled {
		return disabledBackend{}, nil
	}

	switch cfg.Transport {
	case "", "docker-exec":
		return newDockerExecBackend(cfg), nil
	case "gateway":
		return newGatewayBackend(cfg)
	default:
		return nil, fmt.Errorf("unknown fabric transport %q", cfg.Transport)
	}
}

// disabledBackend is used when Fabric sync is disabled.
type disabledBackend struct{}

func (disabledBackend) Invoke(context.Context, string, string, []string) (*InvokeResponse, error) {
	return nil, fmt.Errorf("fabric sync is disabled")
}

func (disabledBackend) Query(context.Context, string, string, []string) (*QueryResponse, error) {
	return nil, fmt.Errorf("fabric sync is disabled")
}

func (disabledBackend) HealthCheck(context.Context) error {
	return fmt.Errorf("fabric sync is disabled")
}

func (disabledBackend) Close() error { return nil }
