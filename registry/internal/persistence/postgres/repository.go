package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/ashrafalaodat/registry/internal/domain"
	"github.com/ashrafalaodat/registry/internal/persistence"
)

// Repository implements persistence against PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// UpsertTool inserts or updates a tool record.
func (r *Repository) UpsertTool(ctx context.Context, tool domain.Tool) (domain.Tool, error) {
	const query = `
        INSERT INTO tools (id, owner, name, description, embedding, inputs, outputs, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, COALESCE($6, '{}'::jsonb), COALESCE($7, '{}'::jsonb), $8, $9)
        ON CONFLICT (id)
        DO UPDATE SET
            owner = EXCLUDED.owner,
            name = EXCLUDED.name,
            description = EXCLUDED.description,
            embedding = EXCLUDED.embedding,
            inputs = EXCLUDED.inputs,
            outputs = EXCLUDED.outputs,
            updated_at = EXCLUDED.updated_at
        RETURNING id, owner, name, description, embedding, inputs, outputs, created_at, updated_at;
    `

	inputs, err := marshalJSON(tool.Inputs)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("marshal inputs: %w", err)
	}
	outputs, err := marshalJSON(tool.Outputs)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("marshal outputs: %w", err)
	}

	var (
		saved          domain.Tool
		savedInputs    []byte
		savedOutputs   []byte
		savedEmbedding scannedVector
	)

	err = r.pool.QueryRow(ctx, query,
		tool.ID,
		tool.Owner,
		tool.Name,
		tool.Description,
		nullableVector(tool.Embedding),
		inputs,
		outputs,
		tool.CreatedAt,
		tool.UpdatedAt,
	).Scan(
		&saved.ID,
		&saved.Owner,
		&saved.Name,
		&saved.Description,
		&savedEmbedding,
		&savedInputs,
		&savedOutputs,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("upsert tool: %w", err)
	}

	saved.Embedding = savedEmbedding.Slice()
	if err := json.Unmarshal(savedInputs, &saved.Inputs); err != nil {
		return domain.Tool{}, fmt.Errorf("unmarshal inputs: %w", err)
	}
	if err := json.Unmarshal(savedOutputs, &saved.Outputs); err != nil {
		return domain.Tool{}, fmt.Errorf("unmarshal outputs: %w", err)
	}

	return saved, nil
}

// GetToolByID fetches a tool record.
func (r *Repository) GetToolByID(ctx context.Context, id uuid.UUID) (domain.Tool, error) {
	const query = `
        SELECT id, owner, name, description, embedding, inputs, outputs, created_at, updated_at
        FROM tools
        WHERE id = $1;
    `

	var (
		tool      domain.Tool
		inputs    []byte
		outputs   []byte
		embedding scannedVector
	)

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&tool.ID,
		&tool.Owner,
		&tool.Name,
		&tool.Description,
		&embedding,
		&inputs,
		&outputs,
		&tool.CreatedAt,
		&tool.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Tool{}, persistence.ErrNotFound
		}
		return domain.Tool{}, fmt.Errorf("query tool: %w", err)
	}

	tool.Embedding = embedding.Slice()
	if err := json.Unmarshal(inputs, &tool.Inputs); err != nil {
		return domain.Tool{}, fmt.Errorf("unmarshal inputs: %w", err)
	}
	if err := json.Unmarshal(outputs, &tool.Outputs); err != nil {
		return domain.Tool{}, fmt.Errorf("unmarshal outputs: %w", err)
	}

	return tool, nil
}

// ListTools returns tools filtered by owner/name/query.
func (r *Repository) ListTools(ctx context.Context, filters persistence.ToolFilters) ([]domain.Tool, error) {
	base := `
        SELECT id, owner, name, description, embedding, inputs, outputs, created_at, updated_at
        FROM tools
    `

	var (
		clauses []string
		args    []any
	)

	if filters.Owner != "" {
		clauses = append(clauses, fmt.Sprintf("owner = $%d", len(args)+1))
		args = append(args, filters.Owner)
	}
	if filters.Name != "" {
		clauses = append(clauses, fmt.Sprintf("name ILIKE $%d", len(args)+1))
		args = append(args, "%"+filters.Name+"%")
	}
	if filters.Query != "" {
		clauses = append(clauses, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", len(args)+1, len(args)+2))
		args = append(args, "%"+filters.Query+"%", "%"+filters.Query+"%")
	}

	query := base
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC;"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	defer rows.Close()

	var tools []domain.Tool
	for rows.Next() {
		var (
			tool      domain.Tool
			inputs    []byte
			outputs   []byte
			embedding scannedVector
		)

		if err := rows.Scan(
			&tool.ID,
			&tool.Owner,
			&tool.Name,
			&tool.Description,
			&embedding,
			&inputs,
			&outputs,
			&tool.CreatedAt,
			&tool.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan tool: %w", err)
		}

		tool.Embedding = embedding.Slice()
		if err := json.Unmarshal(inputs, &tool.Inputs); err != nil {
			return nil, fmt.Errorf("unmarshal inputs: %w", err)
		}
		if err := json.Unmarshal(outputs, &tool.Outputs); err != nil {
			return nil, fmt.Errorf("unmarshal outputs: %w", err)
		}

		tools = append(tools, tool)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}

	return tools, nil
}

// ReplacePolicies replaces the policy set for a tool.
func (r *Repository) ReplacePolicies(ctx context.Context, toolID uuid.UUID, policies []domain.Policy) error {
	const deleteStmt = `DELETE FROM policies WHERE tool_id = $1;`

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, deleteStmt, toolID); err != nil {
		return fmt.Errorf("delete policies: %w", err)
	}

	if len(policies) > 0 {
		batch := &pgx.Batch{}
		const insertStmt = `
            INSERT INTO policies (id, tool_id, principal, allowed_scope, conditions, created_at)
            VALUES ($1, $2, $3, $4, COALESCE($5, '{}'::jsonb), $6);
        `
		for _, policy := range policies {
			conditions, err := marshalJSON(policy.Conditions)
			if err != nil {
				return fmt.Errorf("marshal policy conditions: %w", err)
			}
			batch.Queue(insertStmt,
				policy.ID,
				toolID,
				policy.Principal,
				policy.AllowedScope,
				conditions,
				policy.CreatedAt,
			)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("insert policies: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit policies tx: %w", err)
	}
	return nil
}

// ListPolicies fetches policies for a tool.
func (r *Repository) ListPolicies(ctx context.Context, toolID uuid.UUID) ([]domain.Policy, error) {
	const query = `
        SELECT id, tool_id, principal, allowed_scope, conditions, created_at
        FROM policies
        WHERE tool_id = $1
        ORDER BY created_at DESC;
    `

	rows, err := r.pool.Query(ctx, query, toolID)
	if err != nil {
		return nil, fmt.Errorf("query policies: %w", err)
	}
	defer rows.Close()

	var result []domain.Policy
	for rows.Next() {
		var (
			policy     domain.Policy
			conditions []byte
		)
		if err := rows.Scan(
			&policy.ID,
			&policy.ToolID,
			&policy.Principal,
			&policy.AllowedScope,
			&conditions,
			&policy.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		if err := json.Unmarshal(conditions, &policy.Conditions); err != nil {
			return nil, fmt.Errorf("unmarshal policy conditions: %w", err)
		}
		result = append(result, policy)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}
	return result, nil
}

// CreateAudit writes an audit record.
func (r *Repository) CreateAudit(ctx context.Context, event domain.AuditEvent) (domain.AuditEvent, error) {
	const query = `
        INSERT INTO audit (tool_id, event_type, actor, payload, created_at)
        VALUES ($1, $2, $3, COALESCE($4, '{}'::jsonb), $5)
        RETURNING id;
    `

	payload, err := marshalJSON(event.Payload)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("marshal payload: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query,
		event.ToolID,
		event.EventType,
		event.Actor,
		payload,
		event.CreatedAt,
	).Scan(&event.ID); err != nil {
		return domain.AuditEvent{}, fmt.Errorf("insert audit: %w", err)
	}

	return event, nil
}

// ListAudits returns recent audit entries for a tool.
func (r *Repository) ListAudits(ctx context.Context, toolID uuid.UUID, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 20
	}

	const query = `
        SELECT id, tool_id, event_type, actor, payload, created_at
        FROM audit
        WHERE tool_id = $1
        ORDER BY created_at DESC
        LIMIT $2;
    `

	rows, err := r.pool.Query(ctx, query, toolID, limit)
	if err != nil {
		return nil, fmt.Errorf("query audits: %w", err)
	}
	defer rows.Close()

	var result []domain.AuditEvent
	for rows.Next() {
		var (
			audit   domain.AuditEvent
			payload []byte
		)
		if err := rows.Scan(
			&audit.ID,
			&audit.ToolID,
			&audit.EventType,
			&audit.Actor,
			&payload,
			&audit.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		if err := json.Unmarshal(payload, &audit.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal audit payload: %w", err)
		}
		result = append(result, audit)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}
	return result, nil
}

type scannedVector struct {
	pgvector.Vector
	valid bool
}

func (sv *scannedVector) Scan(src any) error {
	if src == nil {
		sv.valid = false
		sv.Vector = pgvector.Vector{}
		return nil
	}
	if err := sv.Vector.Scan(src); err != nil {
		return err
	}
	sv.valid = true
	return nil
}

func (sv scannedVector) Slice() []float32 {
	if !sv.valid {
		return nil
	}
	return sv.Vector.Slice()
}

func marshalJSON(value any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return []byte("{}"), nil
	}
	return b, nil
}

func nullableVector(values []float32) any {
	if values == nil || len(values) == 0 {
		return nil
	}
	v := pgvector.NewVector(values)
	return v
}
