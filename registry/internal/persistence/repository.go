package persistence

import (
	"context"
	"errors"

	"github.com/ashrafalaodat/smcp/registry/internal/domain"
)

// ErrNotFound indicates that the requested record does not exist.
var ErrNotFound = errors.New("record not found")

// Filters constrains listing queries.
type Filters struct {
	Owner      string
	Name       string
	Visibility string
}

// SearchFilters extend Filters with a limit for similarity queries.
type SearchFilters struct {
	Filters
	Limit int
}

// Repository captures persistence interactions for the registry.
type Repository interface {
	Upsert(ctx context.Context, tool domain.ToolRecord) error
	Get(ctx context.Context, owner, name string) (domain.ToolRecord, error)
	Delete(ctx context.Context, owner, name string) error
	List(ctx context.Context, filters Filters) ([]domain.ToolRecord, error)
	Search(ctx context.Context, embedding []float32, filters SearchFilters) ([]domain.ScoredTool, error)
	Close() error
}
