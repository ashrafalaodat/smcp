package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ashrafalaodat/smcp/registry/internal/domain"
	"github.com/ashrafalaodat/smcp/registry/internal/persistence"
)

var (
	// ErrVectorizerUnavailable indicates that no vectorizer client was configured.
	ErrVectorizerUnavailable = errors.New("vectorizer not configured")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }

func newValidationError(format string, args ...any) error {
	return validationError{msg: fmt.Sprintf(format, args...)}
}

func isValidationError(err error) bool {
	var target validationError
	return errors.As(err, &target)
}

// IsValidationError signals whether the provided error originated from input validation.
func IsValidationError(err error) bool {
	return isValidationError(err)
}

// Vectorizer encapsulates embedding generation for free-form text.
type Vectorizer interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Service coordinates registry operations on top of the repository.
type Service struct {
	repo              persistence.Repository
	vectorizer        Vectorizer
	dim               int
	defaultVisibility string
}

// New creates a service instance.
func New(repo persistence.Repository, vectorizer Vectorizer, dim int, defaultVisibility string) *Service {
	vis := strings.ToLower(strings.TrimSpace(defaultVisibility))
	if vis == "" {
		vis = "public"
	}
	return &Service{
		repo:              repo,
		vectorizer:        vectorizer,
		dim:               dim,
		defaultVisibility: vis,
	}
}

// RegisterToolInput captures the payload for registrations.
type RegisterToolInput struct {
	Owner       string
	Name        string
	Visibility  string
	Description string
}

// ListToolsInput constrains listing queries.
type ListToolsInput struct {
	Owner      string
	Visibility string
}

// SearchToolsInput constrains similarity searches.
type SearchToolsInput struct {
	Owner       string
	Visibility  string
	Limit       int
	Description string
}

// RegisterTool writes or updates a tool record.
func (s *Service) RegisterTool(ctx context.Context, input RegisterToolInput) (domain.ToolRecord, error) {
	owner := strings.TrimSpace(input.Owner)
	name := strings.TrimSpace(input.Name)
	if owner == "" {
		return domain.ToolRecord{}, newValidationError("owner is required")
	}
	if name == "" {
		return domain.ToolRecord{}, newValidationError("name is required")
	}

	embedding, err := s.embedDescription(ctx, input.Description)
	if err != nil {
		return domain.ToolRecord{}, err
	}

	visibility, err := s.normalizeVisibility(input.Visibility)
	if err != nil {
		return domain.ToolRecord{}, err
	}

	record := domain.ToolRecord{
		Owner:      owner,
		Name:       name,
		Visibility: visibility,
		Embedding:  embedding,
	}
	if err := s.repo.Upsert(ctx, record); err != nil {
		return domain.ToolRecord{}, err
	}
	return record, nil
}

// GetTool fetches a tool by owner/name.
func (s *Service) GetTool(ctx context.Context, owner, name string) (domain.ToolRecord, error) {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return domain.ToolRecord{}, newValidationError("owner and name are required")
	}
	return s.repo.Get(ctx, owner, name)
}

// DeleteTool removes the tool identified by owner/name.
func (s *Service) DeleteTool(ctx context.Context, owner, name string) error {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return newValidationError("owner and name are required")
	}
	return s.repo.Delete(ctx, owner, name)
}

// ListTools returns all tools matching the filters.
func (s *Service) ListTools(ctx context.Context, input ListToolsInput) ([]domain.ToolRecord, error) {
	return s.repo.List(ctx, persistence.Filters{
		Owner:      strings.TrimSpace(input.Owner),
		Visibility: strings.TrimSpace(input.Visibility),
	})
}

// SearchTools performs vector similarity search.
func (s *Service) SearchTools(ctx context.Context, input SearchToolsInput) ([]domain.ScoredTool, error) {
	embedding, err := s.embedDescription(ctx, input.Description)
	if err != nil {
		return nil, err
	}
	return s.repo.Search(ctx, embedding, persistence.SearchFilters{
		Filters: persistence.Filters{
			Owner:      strings.TrimSpace(input.Owner),
			Visibility: strings.TrimSpace(input.Visibility),
		},
		Limit: input.Limit,
	})
}

func (s *Service) embedDescription(ctx context.Context, description string) ([]float32, error) {
	if strings.TrimSpace(description) == "" {
		return nil, newValidationError("description is required")
	}
	if s.vectorizer == nil {
		return nil, ErrVectorizerUnavailable
	}
	embedding, err := s.vectorizer.Embed(ctx, description)
	if err != nil {
		return nil, fmt.Errorf("vectorize description: %w", err)
	}
	if len(embedding) != s.dim {
		return nil, fmt.Errorf("vectorizer returned dimension %d but %d required", len(embedding), s.dim)
	}
	return embedding, nil
}

func (s *Service) normalizeVisibility(value string) (string, error) {
	vis := strings.ToLower(strings.TrimSpace(value))
	if vis == "" {
		return s.defaultVisibility, nil
	}
	switch vis {
	case "public", "private":
		return vis, nil
	default:
		return "", newValidationError("visibility must be public or private")
	}
}
