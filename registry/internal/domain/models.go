package domain

import (
	"time"

	"github.com/google/uuid"
)

// Server represents an MCP server registered with the registry.
type Server struct {
	ID         uuid.UUID     `json:"id"`
	Name       string        `json:"name"`
	Endpoint   string        `json:"endpoint"`
	Version    string        `json:"version"`
	Region     string        `json:"region"`
	AuthMethod string        `json:"auth_method"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// Capability describes a capability exposed by an MCP server.
type Capability struct {
	ID        uuid.UUID         `json:"id"`
	ServerID  uuid.UUID         `json:"server_id"`
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Schema    map[string]any    `json:"schema,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// Health stores the current health view of a server.
type Health struct {
	ServerID   uuid.UUID      `json:"server_id"`
	LastSeen   time.Time      `json:"last_seen"`
	Status     string         `json:"status"`
	LatencyMS  int            `json:"latency_ms"`
	ErrorRate  float64        `json:"error_rate"`
	Details    map[string]any `json:"details,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Policy defines access constraints for the server.
type Policy struct {
	ID           uuid.UUID      `json:"id"`
	ServerID     uuid.UUID      `json:"server_id"`
	Principal    string         `json:"principal"`
	AllowedTools []string       `json:"allowed_tools"`
	Conditions   map[string]any `json:"conditions,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AuditEvent represents an event stored for auditing purposes.
type AuditEvent struct {
	ID        int64           `json:"id"`
	ServerID  uuid.UUID       `json:"server_id"`
	ToolName  string          `json:"tool_name"`
	EventType string          `json:"event_type"`
	Actor     string          `json:"actor"`
	Payload   map[string]any  `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// ServerSnapshot aggregates the server details for discovery responses.
type ServerSnapshot struct {
	Server       Server        `json:"server"`
	Capabilities []Capability  `json:"capabilities,omitempty"`
	Policies     []Policy      `json:"policies,omitempty"`
	Health       *Health       `json:"health,omitempty"`
}
