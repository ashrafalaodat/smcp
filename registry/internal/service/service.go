package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ashrafalaodat/registry/internal/domain"
	"github.com/ashrafalaodat/registry/internal/persistence"
)

// ErrToolNotFound indicates the requested tool id does not exist.
var ErrToolNotFound = errors.New("tool not found")

// ErrVectorizerUnavailable signals vector search cannot be performed.
var ErrVectorizerUnavailable = errors.New("vectorizer not configured")

// Vectorizer describes the semantic embedding dependency.
type Vectorizer interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Service coordinates business logic for the registry.
type Service struct {
	repo       persistence.RegistryRepository
	vectorizer Vectorizer
	now        func() time.Time
}

// New constructs a Service.
func New(repo persistence.RegistryRepository, vectorizer Vectorizer) *Service {
	return &Service{
		repo:       repo,
		vectorizer: vectorizer,
		now:        time.Now,
	}
}

// RegisterToolInput carries registration details.
type RegisterToolInput struct {
	ID          *uuid.UUID
	Owner       string
	Name        string
	Description string
	Inputs      map[string]string
	Outputs     map[string]string
	Policies    []PolicyInput
}

// PolicyInput describes an incoming policy payload.
type PolicyInput struct {
	ID           *uuid.UUID
	Principal    string
	AllowedScope []string
	Conditions   map[string]any
}

// ListToolsInput expresses query parameters for discovery.
type ListToolsInput struct {
	Owner string
	Name  string
	Query string
}

type SearchToolsInput struct {
	Description string
	Limit       int
}

// AuditInput captures audit event data.
type AuditInput struct {
	EventType string
	Actor     string
	Payload   map[string]any
}

// RegisterTool registers or updates a tool entry and refreshes its policies.
func (s *Service) RegisterTool(ctx context.Context, input RegisterToolInput) (domain.ToolDetails, error) {
	if input.Owner == "" {
		return domain.ToolDetails{}, fmt.Errorf("owner is required")
	}
	if input.Name == "" {
		return domain.ToolDetails{}, fmt.Errorf("name is required")
	}
	if input.Description == "" {
		return domain.ToolDetails{}, fmt.Errorf("description is required")
	}

	var (
		toolID  uuid.UUID
		current domain.Tool
		err     error
	)
	if input.ID != nil {
		toolID = *input.ID
		current, err = s.repo.GetToolByID(ctx, toolID)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return domain.ToolDetails{}, ErrToolNotFound
			}
			return domain.ToolDetails{}, fmt.Errorf("load existing tool: %w", err)
		}
	} else {
		toolID = uuid.New()
	}

	embedding := current.Embedding
	if s.vectorizer != nil {
		embedding, err = s.vectorizer.Embed(ctx, input.Description)
		if err != nil {
			return domain.ToolDetails{}, fmt.Errorf("vectorize description: %w", err)
		}
	}
	if len(embedding) == 0 {
		return domain.ToolDetails{}, fmt.Errorf("embedding is required")
	}
	now := s.now()
	createdAt := current.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	inputs := input.Inputs
	if inputs == nil {
		inputs = map[string]string{}
	}
	outputs := input.Outputs
	if outputs == nil {
		outputs = map[string]string{}
	}

	tool := domain.Tool{
		ID:          toolID,
		Owner:       input.Owner,
		Name:        input.Name,
		Description: input.Description,
		Embedding:   embedding,
		Inputs:      inputs,
		Outputs:     outputs,
		CreatedAt:   createdAt,
		UpdatedAt:   now,
	}

	saved, err := s.repo.UpsertTool(ctx, tool)
	if err != nil {
		return domain.ToolDetails{}, fmt.Errorf("upsert tool: %w", err)
	}

	policies := make([]domain.Policy, 0, len(input.Policies))
	for _, policyInput := range input.Policies {
		policyID := uuid.New()
		if policyInput.ID != nil {
			policyID = *policyInput.ID
		}
		policies = append(policies, domain.Policy{
			ID:           policyID,
			ToolID:       saved.ID,
			Principal:    policyInput.Principal,
			AllowedScope: policyInput.AllowedScope,
			Conditions:   policyInput.Conditions,
			CreatedAt:    now,
		})
	}
	if err := s.repo.ReplacePolicies(ctx, saved.ID, policies); err != nil {
		return domain.ToolDetails{}, fmt.Errorf("replace policies: %w", err)
	}

	return s.getDetails(ctx, saved.ID)
}

// GetTool returns the tool and accompanying policies/audits.
func (s *Service) GetTool(ctx context.Context, id uuid.UUID) (domain.ToolDetails, error) {
	return s.getDetails(ctx, id)
}

// ListTools returns tools filtered by the provided options.
func (s *Service) ListTools(ctx context.Context, input ListToolsInput) ([]domain.Tool, error) {
	tools, err := s.repo.ListTools(ctx, persistence.ToolFilters{
		Owner: input.Owner,
		Name:  input.Name,
		Query: input.Query,
	})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return tools, nil
}

// SearchTools returns the top-k tools most similar to the provided description.
func (s *Service) SearchTools(ctx context.Context, input SearchToolsInput) ([]domain.Tool, error) {
	description := strings.TrimSpace(input.Description)
	if description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if s.vectorizer == nil {
		return nil, ErrVectorizerUnavailable
	}

	embedding, err := s.vectorizer.Embed(ctx, description)
	if err != nil {
		return nil, fmt.Errorf("vectorize description: %w", err)
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}

	tools, err := s.repo.SearchToolsByEmbedding(ctx, embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("search tools: %w", err)
	}
	return tools, nil
}

// DeleteTool removes a tool and its associated data.
func (s *Service) DeleteTool(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteTool(ctx, id); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return ErrToolNotFound
		}
		return fmt.Errorf("delete tool: %w", err)
	}
	return nil
}

// RecordAudit attaches an audit event to a tool.
func (s *Service) RecordAudit(ctx context.Context, toolID uuid.UUID, input AuditInput) (domain.AuditEvent, error) {
	if input.EventType == "" {
		return domain.AuditEvent{}, fmt.Errorf("event_type is required")
	}
	if input.Actor == "" {
		return domain.AuditEvent{}, fmt.Errorf("actor is required")
	}

	if _, err := s.repo.GetToolByID(ctx, toolID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return domain.AuditEvent{}, ErrToolNotFound
		}
		return domain.AuditEvent{}, fmt.Errorf("load tool: %w", err)
	}

	event := domain.AuditEvent{
		ToolID:    toolID,
		EventType: input.EventType,
		Actor:     input.Actor,
		Payload:   input.Payload,
		CreatedAt: s.now(),
	}

	saved, err := s.repo.CreateAudit(ctx, event)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("create audit: %w", err)
	}
	return saved, nil
}

func (s *Service) getDetails(ctx context.Context, id uuid.UUID) (domain.ToolDetails, error) {
	tool, err := s.repo.GetToolByID(ctx, id)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return domain.ToolDetails{}, ErrToolNotFound
		}
		return domain.ToolDetails{}, fmt.Errorf("get tool: %w", err)
	}

	policies, err := s.repo.ListPolicies(ctx, id)
	if err != nil {
		return domain.ToolDetails{}, fmt.Errorf("list policies: %w", err)
	}

	audits, err := s.repo.ListAudits(ctx, id, 20)
	if err != nil {
		return domain.ToolDetails{}, fmt.Errorf("list audits: %w", err)
	}

	return domain.ToolDetails{
		Tool:     tool,
		Policies: policies,
		Audits:   audits,
	}, nil
}
