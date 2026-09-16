package tunnel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrPluginMissing = errors.New("secure MCP tunnel core plugin is not installed")

func PluginMissingError() error {
	return fmt.Errorf("%w. Repair with: cgm plugin install %s", ErrPluginMissing, PluginIDSecureMCP)
}

type AdminBackend interface {
	FetchMetadata(ctx context.Context, cfg Config) (Metadata, error)
	ListManaged(ctx context.Context, cfg Config, scope AdminScope) ([]Metadata, error)
	GetManaged(ctx context.Context, cfg Config, id string) (Metadata, error)
	CreateManaged(ctx context.Context, cfg Config, req CreateRequest) (Metadata, error)
	UpdateManaged(ctx context.Context, cfg Config, id string, req UpdateRequest) (Metadata, error)
	DeleteManaged(ctx context.Context, cfg Config, id string) (Metadata, error)
	VerifyAdminKey(ctx context.Context, cfg Config) (AdminAccess, int, error)
}

var (
	adminMu       sync.RWMutex
	adminBackend  AdminBackend
	adminResolver func() (AdminBackend, error)
)

func SetAdminBackend(backend AdminBackend) {
	adminMu.Lock()
	adminBackend = backend
	adminMu.Unlock()
}

func SetAdminResolver(resolver func() (AdminBackend, error)) {
	adminMu.Lock()
	adminResolver = resolver
	adminMu.Unlock()
}

func currentAdmin() AdminBackend {
	adminMu.RLock()
	defer adminMu.RUnlock()
	return adminBackend
}

func requireAdmin() (AdminBackend, error) {
	if backend := currentAdmin(); backend != nil {
		return backend, nil
	}
	adminMu.RLock()
	resolver := adminResolver
	adminMu.RUnlock()
	if resolver == nil {
		return nil, PluginMissingError()
	}
	backend, err := resolver()
	if err != nil {
		return nil, err
	}
	SetAdminBackend(backend)
	return backend, nil
}
