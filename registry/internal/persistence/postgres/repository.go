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

// UpsertServer inserts or updates a server record.
func (r *Repository) UpsertServer(ctx context.Context, server domain.Server) (domain.Server, error) {
	const query = `
	INSERT INTO servers (id, name, endpoint, version, region, auth_method, metadata, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, '{}'::jsonb), $8, $9)
	ON CONFLICT (id)
	DO UPDATE SET
		name = EXCLUDED.name,
		endpoint = EXCLUDED.endpoint,
		version = EXCLUDED.version,
		region = EXCLUDED.region,
		auth_method = EXCLUDED.auth_method,
		metadata = EXCLUDED.metadata,
		updated_at = EXCLUDED.updated_at
	RETURNING id, name, endpoint, version, region, auth_method, metadata, created_at, updated_at;
	`

	metadata, err := marshalJSON(server.Metadata)
	if err != nil {
		return domain.Server{}, fmt.Errorf("marshal metadata: %w", err)
	}

	var saved domain.Server
	if err := r.pool.QueryRow(ctx, query,
		server.ID,
		server.Name,
		server.Endpoint,
		server.Version,
		server.Region,
		server.AuthMethod,
		metadata,
		server.CreatedAt,
		server.UpdatedAt,
	).Scan(
		&saved.ID,
		&saved.Name,
		&saved.Endpoint,
		&saved.Version,
		&saved.Region,
		&saved.AuthMethod,
		&metadata,
		&saved.CreatedAt,
		&saved.UpdatedAt,
	); err != nil {
		return domain.Server{}, fmt.Errorf("upsert server: %w", err)
	}

	if err := json.Unmarshal(metadata, &saved.Metadata); err != nil {
		return domain.Server{}, fmt.Errorf("unmarshal metadata: %w", err)
	}

	return saved, nil
}

// ReplaceCapabilities replaces the capability set for a server.
func (r *Repository) ReplaceCapabilities(ctx context.Context, serverID uuid.UUID, capabilities []domain.Capability) error {
	const deleteStmt = `DELETE FROM capabilities WHERE server_id = $1;`
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, deleteStmt, serverID); err != nil {
		return fmt.Errorf("delete capabilities: %w", err)
	}

	if len(capabilities) > 0 {
		batch := &pgx.Batch{}
		const insertStmt = `
			INSERT INTO capabilities (id, server_id, type, name, schema, tags, created_at)
			VALUES ($1, $2, $3, $4, COALESCE($5, '{}'::jsonb), $6, $7);
		`
		for _, cap := range capabilities {
			schema, err := marshalJSON(cap.Schema)
			if err != nil {
				return fmt.Errorf("marshal capability schema: %w", err)
			}
			batch.Queue(insertStmt,
				cap.ID,
				serverID,
				cap.Type,
				cap.Name,
				schema,
				cap.Tags,
				cap.CreatedAt,
			)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("insert capabilities: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit capabilities tx: %w", err)
	}
	return nil
}

// ReplacePolicies replaces the policy set for a server.
func (r *Repository) ReplacePolicies(ctx context.Context, serverID uuid.UUID, policies []domain.Policy) error {
	const deleteStmt = `DELETE FROM policies WHERE server_id = $1;`
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, deleteStmt, serverID); err != nil {
		return fmt.Errorf("delete policies: %w", err)
	}

	if len(policies) > 0 {
		batch := &pgx.Batch{}
		const insertStmt = `
			INSERT INTO policies (id, server_id, principal, allowed_tools, conditions, created_at)
			VALUES ($1, $2, $3, $4, COALESCE($5, '{}'::jsonb), $6);
		`
		for _, policy := range policies {
			conditions, err := marshalJSON(policy.Conditions)
			if err != nil {
				return fmt.Errorf("marshal policy conditions: %w", err)
			}
			batch.Queue(insertStmt,
				policy.ID,
				serverID,
				policy.Principal,
				policy.AllowedTools,
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

// UpsertHealth upserts the health entry for the server.
func (r *Repository) UpsertHealth(ctx context.Context, health domain.Health) error {
	const query = `
	INSERT INTO health (server_id, last_seen, status, latency_ms, error_rate, details, updated_at)
	VALUES ($1, $2, $3, $4, $5, COALESCE($6, '{}'::jsonb), $7)
	ON CONFLICT (server_id)
	DO UPDATE SET
		last_seen = EXCLUDED.last_seen,
		status = EXCLUDED.status,
		latency_ms = EXCLUDED.latency_ms,
		error_rate = EXCLUDED.error_rate,
		details = EXCLUDED.details,
		updated_at = EXCLUDED.updated_at;
	`

	details, err := marshalJSON(health.Details)
	if err != nil {
		return fmt.Errorf("marshal health details: %w", err)
	}

	if _, err := r.pool.Exec(ctx, query,
		health.ServerID,
		health.LastSeen,
		health.Status,
		health.LatencyMS,
		health.ErrorRate,
		details,
		health.UpdatedAt,
	); err != nil {
		return fmt.Errorf("upsert health: %w", err)
	}
	return nil
}

// GetServerByID fetches a server record.
func (r *Repository) GetServerByID(ctx context.Context, id uuid.UUID) (domain.Server, error) {
	const query = `
	SELECT id, name, endpoint, version, region, auth_method, metadata, created_at, updated_at
	FROM servers
	WHERE id = $1;
	`
	var (
		server   domain.Server
		metadata []byte
	)
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&server.ID,
		&server.Name,
		&server.Endpoint,
		&server.Version,
		&server.Region,
		&server.AuthMethod,
		&metadata,
		&server.CreatedAt,
		&server.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Server{}, persistence.ErrNotFound
		}
		return domain.Server{}, fmt.Errorf("query server: %w", err)
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &server.Metadata); err != nil {
			return domain.Server{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	} else {
		server.Metadata = map[string]any{}
	}
	return server, nil
}

// ListServers returns servers filtered by the provided criteria.
func (r *Repository) ListServers(ctx context.Context, filters persistence.Filters) ([]domain.Server, error) {
	base := `
	SELECT s.id, s.name, s.endpoint, s.version, s.region, s.auth_method, s.metadata, s.created_at, s.updated_at
	FROM servers s
	LEFT JOIN health h ON h.server_id = s.id
	`
	var (
		clauses []string
		args    []any
	)
	if filters.Region != "" {
		clauses = append(clauses, fmt.Sprintf("s.region = $%d", len(args)+1))
		args = append(args, filters.Region)
	}
	if filters.Status != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(h.status, 'unknown') = $%d", len(args)+1))
		args = append(args, filters.Status)
	}
	if filters.Tag != "" {
		clauses = append(clauses, fmt.Sprintf("EXISTS (SELECT 1 FROM capabilities c WHERE c.server_id = s.id AND $%d = ANY(c.tags))", len(args)+1))
		args = append(args, filters.Tag)
	}

	query := base
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY s.created_at DESC;"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()

	var servers []domain.Server
	for rows.Next() {
		var (
			server   domain.Server
			metadata []byte
		)
		if err := rows.Scan(
			&server.ID,
			&server.Name,
			&server.Endpoint,
			&server.Version,
			&server.Region,
			&server.AuthMethod,
			&metadata,
			&server.CreatedAt,
			&server.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &server.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		} else {
			server.Metadata = map[string]any{}
		}
		servers = append(servers, server)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}

	return servers, nil
}

// GetCapabilities fetches capability records for a server.
func (r *Repository) GetCapabilities(ctx context.Context, serverID uuid.UUID) ([]domain.Capability, error) {
	const query = `
	SELECT id, server_id, type, name, schema, tags, created_at
	FROM capabilities
	WHERE server_id = $1
	ORDER BY created_at DESC;
	`
	rows, err := r.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, fmt.Errorf("query capabilities: %w", err)
	}
	defer rows.Close()

	var result []domain.Capability
	for rows.Next() {
		var (
			capability domain.Capability
			schema     []byte
		)
		if err := rows.Scan(
			&capability.ID,
			&capability.ServerID,
			&capability.Type,
			&capability.Name,
			&schema,
			&capability.Tags,
			&capability.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan capability: %w", err)
		}
		if len(schema) > 0 {
			if err := json.Unmarshal(schema, &capability.Schema); err != nil {
				return nil, fmt.Errorf("unmarshal capability schema: %w", err)
			}
		}
		result = append(result, capability)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}
	return result, nil
}

// GetPolicies fetches policies for a server.
func (r *Repository) GetPolicies(ctx context.Context, serverID uuid.UUID) ([]domain.Policy, error) {
	const query = `
	SELECT id, server_id, principal, allowed_tools, conditions, created_at
	FROM policies
	WHERE server_id = $1
	ORDER BY created_at DESC;
	`
	rows, err := r.pool.Query(ctx, query, serverID)
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
			&policy.ServerID,
			&policy.Principal,
			&policy.AllowedTools,
			&conditions,
			&policy.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		if len(conditions) > 0 {
			if err := json.Unmarshal(conditions, &policy.Conditions); err != nil {
				return nil, fmt.Errorf("unmarshal policy conditions: %w", err)
			}
		}
		result = append(result, policy)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}
	return result, nil
}

// GetHealth fetches the health record for a server.
func (r *Repository) GetHealth(ctx context.Context, serverID uuid.UUID) (*domain.Health, error) {
	const query = `
	SELECT server_id, last_seen, status, latency_ms, error_rate, details, updated_at
	FROM health
	WHERE server_id = $1;
	`
	var (
		health  domain.Health
		details []byte
	)
	err := r.pool.QueryRow(ctx, query, serverID).Scan(
		&health.ServerID,
		&health.LastSeen,
		&health.Status,
		&health.LatencyMS,
		&health.ErrorRate,
		&details,
		&health.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query health: %w", err)
	}
	if len(details) > 0 {
		if err := json.Unmarshal(details, &health.Details); err != nil {
			return nil, fmt.Errorf("unmarshal health details: %w", err)
		}
	}
	return &health, nil
}

func marshalJSON(value map[string]any) ([]byte, error) {
	if value == nil || len(value) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return b, nil
}
