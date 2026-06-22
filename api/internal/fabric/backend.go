package fabric

import (
	"context"
	"fmt"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

// Backend is the transport used to talk to the Fabric peer — the Fabric Gateway
// gRPC SDK. Selecting the backend is a configuration concern, so the rest of the
// API depends only on this interface.
type Backend interface {
	Invoke(ctx context.Context, channel, function string, args []string) (*InvokeResponse, error)
	Query(ctx context.Context, channel, function string, args []string) (*QueryResponse, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

var (
	_ Backend = (*gatewayBackend)(nil)
	_ Backend = disabledBackend{}
)

// newBackend constructs the Fabric transport. Only the gateway transport is
// supported (the legacy docker-exec transport, which required mounting the
// Docker socket, was removed). When sync is disabled a no-op backend is returned
// so the client constructs cleanly without connectivity or credentials.
func newBackend(cfg *config.FabricConfig) (Backend, error) {
	if !cfg.SyncEnabled {
		return disabledBackend{}, nil
	}

	switch cfg.Transport {
	case "", "gateway":
		return newGatewayBackend(cfg)
	default:
		return nil, fmt.Errorf("unsupported fabric transport %q (only \"gateway\" is supported)", cfg.Transport)
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
