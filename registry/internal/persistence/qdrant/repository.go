package qdrantrepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	qdrantsdk "github.com/qdrant/go-client/qdrant"

	"github.com/ashrafalaodat/smcp/registry/internal/domain"
	"github.com/ashrafalaodat/smcp/registry/internal/persistence"
)

const (
	payloadOwnerKey      = "owner"
	payloadToolNameKey   = "tool_name"
	payloadVisibilityKey = "visibility"
)

// Config captures the dependencies required to connect to Qdrant.
type Config struct {
	Host       string
	Port       int
	APIKey     string
	Collection string
	Dimension  int
}

// Repository persists tool vectors inside Qdrant.
type Repository struct {
	client     *qdrantsdk.Client
	collection string
	dim        int
}

// New creates a Qdrant-backed repository.
func New(ctx context.Context, cfg Config) (*Repository, error) {
	if cfg.Dimension <= 0 {
		return nil, fmt.Errorf("dimension must be positive")
	}
	collection := cfg.Collection
	if strings.TrimSpace(collection) == "" {
		collection = "mcp_tools"
	}
	client, err := qdrantsdk.NewClient(&qdrantsdk.Config{
		Host:   cfg.Host,
		Port:   cfg.Port,
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create qdrant client: %w", err)
	}

	repo := &Repository{client: client, collection: collection, dim: cfg.Dimension}
	if err := repo.ensureCollection(ctx); err != nil {
		client.Close()
		return nil, err
	}
	return repo, nil
}

// Close releases the underlying gRPC connections.
func (r *Repository) Close() error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}

// Upsert inserts or updates a tool embedding.
func (r *Repository) Upsert(ctx context.Context, tool domain.ToolRecord) error {
	if err := r.validateEmbedding(tool.Embedding); err != nil {
		return err
	}

	point := &qdrantsdk.PointStruct{
		Id:      qdrantsdk.NewID(toolIdentifier(tool.Owner, tool.Name)),
		Vectors: qdrantsdk.NewVectorsDense(append([]float32(nil), tool.Embedding...)),
		Payload: qdrantsdk.NewValueMap(map[string]any{
			payloadOwnerKey:      tool.Owner,
			payloadToolNameKey:   tool.Name,
			payloadVisibilityKey: tool.Visibility,
		}),
	}

	wait := true
	if _, err := r.client.Upsert(ctx, &qdrantsdk.UpsertPoints{
		CollectionName: r.collection,
		Wait:           &wait,
		Points:         []*qdrantsdk.PointStruct{point},
	}); err != nil {
		return fmt.Errorf("qdrant upsert: %w", err)
	}
	return nil
}

// Get retrieves a tool by owner/name.
func (r *Repository) Get(ctx context.Context, owner, name string) (domain.ToolRecord, error) {
	points, err := r.client.Get(ctx, &qdrantsdk.GetPoints{
		CollectionName: r.collection,
		Ids:            []*qdrantsdk.PointId{qdrantsdk.NewID(toolIdentifier(owner, name))},
		WithPayload:    qdrantsdk.NewWithPayload(true),
		WithVectors:    qdrantsdk.NewWithVectors(true),
	})
	if err != nil {
		return domain.ToolRecord{}, fmt.Errorf("qdrant get: %w", err)
	}
	if len(points) == 0 {
		return domain.ToolRecord{}, persistence.ErrNotFound
	}

	record, err := r.recordFromPoint(points[0].GetPayload(), points[0].GetVectors())
	if err != nil {
		return domain.ToolRecord{}, err
	}
	return record, nil
}

// Delete removes a tool from the collection.
func (r *Repository) Delete(ctx context.Context, owner, name string) error {
	wait := true
	_, err := r.client.Delete(ctx, &qdrantsdk.DeletePoints{
		CollectionName: r.collection,
		Wait:           &wait,
		Points: qdrantsdk.NewPointsSelector(
			qdrantsdk.NewID(toolIdentifier(owner, name)),
		),
	})
	if err != nil {
		return fmt.Errorf("qdrant delete: %w", err)
	}
	return nil
}

// List returns all tools that match the provided filters.
func (r *Repository) List(ctx context.Context, filters persistence.Filters) ([]domain.ToolRecord, error) {
	var (
		limit  = uint32(256)
		offset *qdrantsdk.PointId
		tools  []domain.ToolRecord
	)

	for {
		results, next, err := r.client.ScrollAndOffset(ctx, &qdrantsdk.ScrollPoints{
			CollectionName: r.collection,
			Filter:         buildFilter(filters),
			Offset:         offset,
			Limit:          &limit,
			WithPayload:    qdrantsdk.NewWithPayload(true),
			WithVectors:    qdrantsdk.NewWithVectors(true),
		})
		if err != nil {
			return nil, fmt.Errorf("qdrant scroll: %w", err)
		}
		for _, point := range results {
			record, err := r.recordFromPoint(point.GetPayload(), point.GetVectors())
			if err != nil {
				continue
			}
			tools = append(tools, record)
		}
		if next == nil {
			break
		}
		offset = next
	}

	return tools, nil
}

// Search performs a similarity search constrained by filters.
func (r *Repository) Search(ctx context.Context, embedding []float32, filters persistence.SearchFilters) ([]domain.ScoredTool, error) {
	if err := r.validateEmbedding(embedding); err != nil {
		return nil, err
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 5
	}
	limitPtr := qdrantsdk.PtrOf(uint64(limit))

	points, err := r.client.Query(ctx, &qdrantsdk.QueryPoints{
		CollectionName: r.collection,
		Query:          qdrantsdk.NewQueryDense(append([]float32(nil), embedding...)),
		Filter:         buildFilter(filters.Filters),
		Limit:          limitPtr,
		WithVectors:    qdrantsdk.NewWithVectors(true),
		WithPayload:    qdrantsdk.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant query: %w", err)
	}

	results := make([]domain.ScoredTool, 0, len(points))
	for _, point := range points {
		record, err := r.recordFromPoint(point.GetPayload(), point.GetVectors())
		if err != nil {
			continue
		}
		results = append(results, domain.ScoredTool{
			Tool:  record,
			Score: point.GetScore(),
		})
	}
	return results, nil
}

func (r *Repository) ensureCollection(ctx context.Context) error {
	exists, err := r.client.CollectionExists(ctx, r.collection)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if exists {
		info, err := r.client.GetCollectionInfo(ctx, r.collection)
		if err != nil {
			return fmt.Errorf("describe collection: %w", err)
		}
		params := info.GetConfig().GetParams().GetVectorsConfig().GetParams()
		if params == nil || int(params.GetSize()) != r.dim {
			return fmt.Errorf("collection %s vector size %d does not match required %d", r.collection, params.GetSize(), r.dim)
		}
		return nil
	}

	defaultSegments := uint64(2)
	createReq := &qdrantsdk.CreateCollection{
		CollectionName: r.collection,
		VectorsConfig: qdrantsdk.NewVectorsConfig(&qdrantsdk.VectorParams{
			Size:     uint64(r.dim),
			Distance: qdrantsdk.Distance_Cosine,
			HnswConfig: &qdrantsdk.HnswConfigDiff{
				M:           qdrantsdk.PtrOf(uint64(32)),
				EfConstruct: qdrantsdk.PtrOf(uint64(256)),
			},
			QuantizationConfig: &qdrantsdk.QuantizationConfig{
				Quantization: &qdrantsdk.QuantizationConfig_Product{
					Product: &qdrantsdk.ProductQuantization{
						Compression: qdrantsdk.CompressionRatio_x8,
					},
				},
			},
		}),
		OptimizersConfig: &qdrantsdk.OptimizersConfigDiff{
			DefaultSegmentNumber: &defaultSegments,
			IndexingThreshold:    qdrantsdk.PtrOf(uint64(20000)),
		},
	}

	if err := r.client.CreateCollection(ctx, createReq); err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (r *Repository) validateEmbedding(embedding []float32) error {
	if len(embedding) != r.dim {
		return fmt.Errorf("embedding dimension %d does not match configured %d", len(embedding), r.dim)
	}
	return nil
}

func (r *Repository) recordFromPoint(payload map[string]*qdrantsdk.Value, vectors *qdrantsdk.VectorsOutput) (domain.ToolRecord, error) {
	owner := valueToString(payload[payloadOwnerKey])
	name := valueToString(payload[payloadToolNameKey])
	visibility := valueToString(payload[payloadVisibilityKey])
	embedding := vectorFromOutput(vectors)

	if owner == "" || name == "" {
		return domain.ToolRecord{}, fmt.Errorf("point missing identity payload")
	}
	if len(embedding) != r.dim {
		return domain.ToolRecord{}, fmt.Errorf("point embedding size mismatch")
	}

	return domain.ToolRecord{
		Owner:      owner,
		Name:       name,
		Visibility: visibility,
		Embedding:  embedding,
	}, nil
}

func buildFilter(filters persistence.Filters) *qdrantsdk.Filter {
	var conditions []*qdrantsdk.Condition
	if owner := strings.TrimSpace(filters.Owner); owner != "" {
		conditions = append(conditions, qdrantsdk.NewMatch(payloadOwnerKey, owner))
	}
	if name := strings.TrimSpace(filters.Name); name != "" {
		conditions = append(conditions, qdrantsdk.NewMatch(payloadToolNameKey, name))
	}
	if visibility := strings.TrimSpace(filters.Visibility); visibility != "" {
		conditions = append(conditions, qdrantsdk.NewMatch(payloadVisibilityKey, visibility))
	}
	if len(conditions) == 0 {
		return nil
	}
	return &qdrantsdk.Filter{Must: conditions}
}

func toolIdentifier(owner, name string) string {
	key := strings.ToLower(strings.TrimSpace(owner)) + "::" + strings.ToLower(strings.TrimSpace(name))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func valueToString(v *qdrantsdk.Value) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(v.GetStringValue())
}

func vectorFromOutput(vectors *qdrantsdk.VectorsOutput) []float32 {
	if vectors == nil {
		return nil
	}

	if vec := vectors.GetVector(); vec != nil {
		if dense := vec.GetDenseVector(); dense != nil {
			data := dense.GetData()
			return append([]float32(nil), data...)
		}
		if dense := vec.GetDense(); dense != nil {
			data := dense.GetData()
			return append([]float32(nil), data...)
		}
		if legacy := vec.GetData(); len(legacy) > 0 {
			return append([]float32(nil), legacy...)
		}
	}

	if named := vectors.GetVectors(); named != nil {
		for _, vec := range named.GetVectors() {
			if dense := vec.GetDenseVector(); dense != nil {
				return append([]float32(nil), dense.GetData()...)
			}
			if dense := vec.GetDense(); dense != nil {
				return append([]float32(nil), dense.GetData()...)
			}
		}
	}

	return nil
}
