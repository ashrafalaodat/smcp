package persistence

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/ashrafalaodat/registry/internal/domain"
)

// ErrNotFound is returned when an entity cannot be located.
var ErrNotFound = errors.New("not found")

// Filters narrow server lookup results.
type Filters struct {
	Region string
	Status string
	Tag    string
}

// RegistryRepository captures the persistence contract for the registry service.
type RegistryRepository interface {
	UpsertServer(ctx context.Context, server domain.Server) (domain.Server, error)
	ReplaceCapabilities(ctx context.Context, serverID uuid.UUID, capabilities []domain.Capability) error
	ReplacePolicies(ctx context.Context, serverID uuid.UUID, policies []domain.Policy) error
	UpsertHealth(ctx context.Context, health domain.Health) error
	GetServerByID(ctx context.Context, id uuid.UUID) (domain.Server, error)
	ListServers(ctx context.Context, filters Filters) ([]domain.Server, error)
	GetCapabilities(ctx context.Context, serverID uuid.UUID) ([]domain.Capability, error)
	GetPolicies(ctx context.Context, serverID uuid.UUID) ([]domain.Policy, error)
	GetHealth(ctx context.Context, serverID uuid.UUID) (*domain.Health, error)
}
