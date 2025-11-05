package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ashrafalaodat/registry/internal/domain"
	"github.com/ashrafalaodat/registry/internal/persistence"
)

func TestSearchToolsWithoutReranker(t *testing.T) {
	repo := &stubRepository{
		searchResults: []domain.Tool{
			{
				ID:          uuid.New(),
				Owner:       "team-a",
				Name:        "tool-a",
				Description: "first",
				Embedding:   []float32{0.1},
			},
			{
				ID:          uuid.New(),
				Owner:       "team-b",
				Name:        "tool-b",
				Description: "second",
				Embedding:   []float32{0.2},
			},
		},
	}

	svc := New(repo, stubVectorizer{}, nil)

	results, err := svc.SearchTools(context.Background(), SearchToolsInput{
		Description: "query",
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("SearchTools returned error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Name != "tool-a" || results[1].Name != "tool-b" {
		t.Fatalf("unexpected ordering: %+v", results)
	}

	if repo.lastLimit != 6 {
		t.Fatalf("expected repo limit 6, got %d", repo.lastLimit)
	}
}

func TestSearchToolsWithReranker(t *testing.T) {
	toolA := domain.Tool{
		ID:          uuid.New(),
		Owner:       "team-a",
		Name:        "tool-a",
		Description: "first",
		Embedding:   []float32{0.1},
	}
	toolB := domain.Tool{
		ID:          uuid.New(),
		Owner:       "team-b",
		Name:        "tool-b",
		Description: "second",
		Embedding:   []float32{0.2},
	}

	repo := &stubRepository{
		searchResults: []domain.Tool{toolA, toolB},
	}

	reranker := stubReranker{
		order: []int{1, 0},
	}

	svc := New(repo, stubVectorizer{}, reranker)

	results, err := svc.SearchTools(context.Background(), SearchToolsInput{
		Description: "query",
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("SearchTools returned error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Name != "tool-b" || results[1].Name != "tool-a" {
		t.Fatalf("expected reranker to reorder results, got %+v", results)
	}

	if repo.lastLimit != 6 {
		t.Fatalf("expected repo limit 6, got %d", repo.lastLimit)
	}
}

type stubVectorizer struct{}

func (stubVectorizer) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1.0}, nil
}

type stubReranker struct {
	order []int
	err   error
}

func (s stubReranker) Rank(ctx context.Context, query string, candidates []domain.Tool) ([]domain.Tool, error) {
	if s.err != nil {
		return nil, s.err
	}

	seen := make(map[int]bool)
	ranked := make([]domain.Tool, 0, len(candidates))

	for _, idx := range s.order {
		if idx < 0 || idx >= len(candidates) {
			continue
		}
		tool := candidates[idx]
		ranked = append(ranked, tool)
		seen[idx] = true
	}

	for idx, tool := range candidates {
		if seen[idx] {
			continue
		}
		ranked = append(ranked, tool)
	}

	return ranked, nil
}

type stubRepository struct {
	searchResults []domain.Tool
	lastLimit     int
}

func (s *stubRepository) UpsertTool(context.Context, domain.Tool) (domain.Tool, error) {
	return domain.Tool{}, errors.New("not implemented")
}

func (s *stubRepository) GetToolByID(context.Context, uuid.UUID) (domain.Tool, error) {
	return domain.Tool{}, errors.New("not implemented")
}

func (s *stubRepository) ListTools(context.Context, persistence.ToolFilters) ([]domain.Tool, error) {
	return nil, errors.New("not implemented")
}

func (s *stubRepository) SearchToolsByEmbedding(ctx context.Context, embedding []float32, limit int) ([]domain.Tool, error) {
	s.lastLimit = limit
	if len(s.searchResults) == 0 {
		return nil, nil
	}
	results := make([]domain.Tool, len(s.searchResults))
	copy(results, s.searchResults)
	return results, nil
}

func (s *stubRepository) DeleteTool(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}

func (s *stubRepository) ReplacePolicies(context.Context, uuid.UUID, []domain.Policy) error {
	return errors.New("not implemented")
}

func (s *stubRepository) ListPolicies(context.Context, uuid.UUID) ([]domain.Policy, error) {
	return nil, errors.New("not implemented")
}

func (s *stubRepository) CreateAudit(context.Context, domain.AuditEvent) (domain.AuditEvent, error) {
	return domain.AuditEvent{}, errors.New("not implemented")
}

func (s *stubRepository) ListAudits(context.Context, uuid.UUID, int) ([]domain.AuditEvent, error) {
	return nil, errors.New("not implemented")
}
