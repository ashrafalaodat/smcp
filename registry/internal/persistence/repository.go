package persistence

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/ashrafalaodat/registry/internal/domain"
)

// ErrNotFound is returned when an entity cannot be located.
var ErrNotFound = errors.New("not found")

// ToolFilters narrow tool lookup results.
type ToolFilters struct {
	Owner string
	Name  string
	Query string
}

// RegistryRepository captures the persistence contract for the registry service.
type RegistryRepository interface {
	UpsertTool(ctx context.Context, tool domain.Tool) (domain.Tool, error)
	GetToolByID(ctx context.Context, id uuid.UUID) (domain.Tool, error)
	ListTools(ctx context.Context, filters ToolFilters) ([]domain.Tool, error)
	SearchToolsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]domain.Tool, error)
	DeleteTool(ctx context.Context, id uuid.UUID) error

	ReplacePolicies(ctx context.Context, toolID uuid.UUID, policies []domain.Policy) error
	ListPolicies(ctx context.Context, toolID uuid.UUID) ([]domain.Policy, error)

	CreateAudit(ctx context.Context, event domain.AuditEvent) (domain.AuditEvent, error)
	ListAudits(ctx context.Context, toolID uuid.UUID, limit int) ([]domain.AuditEvent, error)
}
