package fabric

import (
	"context"
	"fmt"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

// Backend is the transport used to talk to the Fabric peer. The docker-exec
// backend shells out to the peer CLI; the gateway backend (next) will use the
// Fabric Gateway gRPC SDK. Selecting the backend is a configuration concern, so
// the rest of the API depends only on this interface.
type Backend interface {
	Invoke(ctx context.Context, function string, args []string) (*InvokeResponse, error)
	Query(ctx context.Context, function string, args []string) (*QueryResponse, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

var _ Backend = (*dockerExecBackend)(nil)

// newBackend selects the Fabric transport backend from configuration.
func newBackend(cfg *config.FabricConfig) (Backend, error) {
	switch cfg.Transport {
	case "", "docker-exec":
		return newDockerExecBackend(cfg), nil
	case "gateway":
		return nil, fmt.Errorf("fabric transport %q is not implemented yet", cfg.Transport)
	default:
		return nil, fmt.Errorf("unknown fabric transport %q", cfg.Transport)
	}
}
