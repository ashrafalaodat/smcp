package domain

import (
	"time"

	"github.com/google/uuid"
)

// Tool models a discoverable MCP tool entry.
type Tool struct {
	ID          uuid.UUID         `json:"id"`
	Owner       string            `json:"owner"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Embedding   []float32         `json:"embedding"`
	Inputs      map[string]string `json:"inputs"`
	Outputs     map[string]string `json:"outputs"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Policy defines access constraints for a tool.
type Policy struct {
	ID           uuid.UUID      `json:"id"`
	ToolID       uuid.UUID      `json:"tool_id"`
	Principal    string         `json:"principal"`
	AllowedScope []string       `json:"allowed_scope"`
	Conditions   map[string]any `json:"conditions,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AuditEvent represents an event stored for auditing purposes.
type AuditEvent struct {
	ID        int64          `json:"id"`
	ToolID    uuid.UUID      `json:"tool_id"`
	EventType string         `json:"event_type"`
	Actor     string         `json:"actor"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// ToolDetails aggregates a tool with attached policies and audits.
type ToolDetails struct {
	Tool     Tool         `json:"tool"`
	Policies []Policy     `json:"policies"`
	Audits   []AuditEvent `json:"audits"`
}
