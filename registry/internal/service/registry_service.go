package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ashrafalaodat/registry/internal/domain"
	"github.com/ashrafalaodat/registry/internal/persistence"
)

var (
	// ErrServerNotFound indicates the requested server id does not exist.
	ErrServerNotFound = errors.New("server not found")
)

// Service coordinates business logic for the registry.
type Service struct {
	repo persistence.RegistryRepository
	now  func() time.Time
}

// New constructs a Service.
func New(repo persistence.RegistryRepository) *Service {
	return &Service{
		repo: repo,
		now:  time.Now,
	}
}

// RegisterServerInput carries registration details.
type RegisterServerInput struct {
	ID           *uuid.UUID
	Name         string
	Endpoint     string
	Version      string
	Region       string
	AuthMethod   string
	Metadata     map[string]any
	Capabilities []CapabilityInput
	Policies     []PolicyInput
}

// CapabilityInput describes an incoming capability payload.
type CapabilityInput struct {
	ID     *uuid.UUID
	Type   string
	Name   string
	Schema map[string]any
	Tags   []string
}

// PolicyInput describes an incoming policy payload.
type PolicyInput struct {
	ID           *uuid.UUID
	Principal    string
	AllowedTools []string
	Conditions   map[string]any
}

// HeartbeatInput captures heartbeat data from running MCP instances.
type HeartbeatInput struct {
	Status    string
	LatencyMS int
	ErrorRate float64
	Details   map[string]any
}

// ListServersInput expresses query parameters for discovery.
type ListServersInput struct {
	Region string
	Status string
	Tag    string
}

// RegisterServer registers or updates an MCP server entry.
func (s *Service) RegisterServer(ctx context.Context, input RegisterServerInput) (domain.ServerSnapshot, error) {
	if input.Name == "" {
		return domain.ServerSnapshot{}, fmt.Errorf("name is required")
	}
	if input.Endpoint == "" {
		return domain.ServerSnapshot{}, fmt.Errorf("endpoint is required")
	}

	id := uuid.New()
	if input.ID != nil {
		id = *input.ID
	}
	now := s.now()

	server := domain.Server{
		ID:         id,
		Name:       input.Name,
		Endpoint:   input.Endpoint,
		Version:    input.Version,
		Region:     input.Region,
		AuthMethod: input.AuthMethod,
		Metadata:   input.Metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	saved, err := s.repo.UpsertServer(ctx, server)
	if err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("upsert server: %w", err)
	}

	capabilities := make([]domain.Capability, 0, len(input.Capabilities))
	for _, capInput := range input.Capabilities {
		capID := uuid.New()
		if capInput.ID != nil {
			capID = *capInput.ID
		}
		capabilities = append(capabilities, domain.Capability{
			ID:        capID,
			ServerID:  saved.ID,
			Type:      capInput.Type,
			Name:      capInput.Name,
			Schema:    capInput.Schema,
			Tags:      capInput.Tags,
			CreatedAt: now,
		})
	}
	if err := s.repo.ReplaceCapabilities(ctx, saved.ID, capabilities); err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("replace capabilities: %w", err)
	}

	policies := make([]domain.Policy, 0, len(input.Policies))
	for _, policyInput := range input.Policies {
		policyID := uuid.New()
		if policyInput.ID != nil {
			policyID = *policyInput.ID
		}
		policies = append(policies, domain.Policy{
			ID:           policyID,
			ServerID:     saved.ID,
			Principal:    policyInput.Principal,
			AllowedTools: policyInput.AllowedTools,
			Conditions:   policyInput.Conditions,
			CreatedAt:    now,
		})
	}
	if err := s.repo.ReplacePolicies(ctx, saved.ID, policies); err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("replace policies: %w", err)
	}

	health := domain.Health{
		ServerID:  saved.ID,
		LastSeen:  now,
		Status:    "online",
		LatencyMS: 0,
		ErrorRate: 0,
		UpdatedAt: now,
	}
	if err := s.repo.UpsertHealth(ctx, health); err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("upsert health: %w", err)
	}

	return s.getSnapshot(ctx, saved.ID, saved)
}

// ReportHeartbeat updates server health.
func (s *Service) ReportHeartbeat(ctx context.Context, serverID uuid.UUID, input HeartbeatInput) (domain.ServerSnapshot, error) {
	if _, err := s.repo.GetServerByID(ctx, serverID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return domain.ServerSnapshot{}, ErrServerNotFound
		}
		return domain.ServerSnapshot{}, fmt.Errorf("locate server: %w", err)
	}

	now := s.now()
	health := domain.Health{
		ServerID:  serverID,
		LastSeen:  now,
		Status:    input.Status,
		LatencyMS: input.LatencyMS,
		ErrorRate: input.ErrorRate,
		Details:   input.Details,
		UpdatedAt: now,
	}
	if health.Status == "" {
		health.Status = "online"
	}

	if err := s.repo.UpsertHealth(ctx, health); err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("update health: %w", err)
	}

	return s.getSnapshot(ctx, serverID, domain.Server{})
}

// ListServers returns server snapshots filtered by the provided options.
func (s *Service) ListServers(ctx context.Context, input ListServersInput) ([]domain.ServerSnapshot, error) {
	servers, err := s.repo.ListServers(ctx, persistence.Filters{
		Region: input.Region,
		Status: input.Status,
		Tag:    input.Tag,
	})
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}

	snapshots := make([]domain.ServerSnapshot, 0, len(servers))
	for _, server := range servers {
		snapshot, err := s.getSnapshot(ctx, server.ID, server)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func (s *Service) getSnapshot(ctx context.Context, id uuid.UUID, server domain.Server) (domain.ServerSnapshot, error) {
	var err error
	if server.ID == uuid.Nil {
		server, err = s.repo.GetServerByID(ctx, id)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				return domain.ServerSnapshot{}, ErrServerNotFound
			}
			return domain.ServerSnapshot{}, fmt.Errorf("get server: %w", err)
		}
	}

	capabilities, err := s.repo.GetCapabilities(ctx, id)
	if err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("get capabilities: %w", err)
	}

	policies, err := s.repo.GetPolicies(ctx, id)
	if err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("get policies: %w", err)
	}

	health, err := s.repo.GetHealth(ctx, id)
	if err != nil {
		return domain.ServerSnapshot{}, fmt.Errorf("get health: %w", err)
	}

	return domain.ServerSnapshot{
		Server:       server,
		Capabilities: capabilities,
		Policies:     policies,
		Health:       health,
	}, nil
}
