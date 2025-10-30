package httpapi

import (
    "encoding/json"
    "errors"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "go.uber.org/zap"

    "github.com/ashrafalaodat/registry/internal/domain"
    "github.com/ashrafalaodat/registry/internal/service"
)

// Handler wraps HTTP endpoints for the registry.
type Handler struct {
    service *service.Service
    logger  *zap.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *service.Service, logger *zap.Logger) *Handler {
    return &Handler{
        service: svc,
        logger:  logger,
    }
}

// RegisterRoutes wires the registry routes onto the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Route("/v1", func(r chi.Router) {
        r.Post("/tools", h.handleRegisterTool)
        r.Get("/tools", h.handleListTools)
        r.Get("/tools/{id}", h.handleGetTool)
        r.Post("/tools/{id}/audits", h.handleCreateAudit)
    })
}

func (h *Handler) handleRegisterTool(w http.ResponseWriter, r *http.Request) {
    var payload registerToolRequest
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        h.writeError(w, http.StatusBadRequest, "invalid payload", err)
        return
    }

    var toolID *uuid.UUID
    if payload.ID != "" {
        id, err := uuid.Parse(payload.ID)
        if err != nil {
            h.writeError(w, http.StatusBadRequest, "invalid id", err)
            return
        }
        toolID = &id
    }

    input := service.RegisterToolInput{
        ID:          toolID,
        Owner:       payload.Owner,
        Name:        payload.Name,
        Description: payload.Description,
        Inputs:      payload.Inputs,
        Outputs:     payload.Outputs,
    }

    for _, policy := range payload.Policies {
        var policyID *uuid.UUID
        if policy.ID != "" {
            id, err := uuid.Parse(policy.ID)
            if err != nil {
                h.writeError(w, http.StatusBadRequest, "invalid policy id", err)
                return
            }
            policyID = &id
        }
        input.Policies = append(input.Policies, service.PolicyInput{
            ID:           policyID,
            Principal:    policy.Principal,
            AllowedScope: policy.AllowedScope,
            Conditions:   policy.Conditions,
        })
    }

    details, err := h.service.RegisterTool(r.Context(), input)
    if err != nil {
        status := http.StatusInternalServerError
        if errors.Is(err, service.ErrToolNotFound) {
            status = http.StatusNotFound
        }
        h.writeError(w, status, "register tool failed", err)
        return
    }

    h.writeJSON(w, http.StatusCreated, toToolDetailsPayload(details))
}

func (h *Handler) handleListTools(w http.ResponseWriter, r *http.Request) {
    input := service.ListToolsInput{
        Owner: r.URL.Query().Get("owner"),
        Name:  r.URL.Query().Get("name"),
        Query: r.URL.Query().Get("q"),
    }

    tools, err := h.service.ListTools(r.Context(), input)
    if err != nil {
        h.writeError(w, http.StatusInternalServerError, "list tools failed", err)
        return
    }

    payload := make([]toolPayload, 0, len(tools))
    for _, tool := range tools {
        payload = append(payload, toToolPayload(tool))
    }
    h.writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleGetTool(w http.ResponseWriter, r *http.Request) {
    id, err := uuid.Parse(chi.URLParam(r, "id"))
    if err != nil {
        h.writeError(w, http.StatusBadRequest, "invalid id", err)
        return
    }

    details, err := h.service.GetTool(r.Context(), id)
    if err != nil {
        status := http.StatusInternalServerError
        if errors.Is(err, service.ErrToolNotFound) {
            status = http.StatusNotFound
        }
        h.writeError(w, status, "get tool failed", err)
        return
    }

    h.writeJSON(w, http.StatusOK, toToolDetailsPayload(details))
}

func (h *Handler) handleCreateAudit(w http.ResponseWriter, r *http.Request) {
    id, err := uuid.Parse(chi.URLParam(r, "id"))
    if err != nil {
        h.writeError(w, http.StatusBadRequest, "invalid id", err)
        return
    }

    var payload auditRequest
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        h.writeError(w, http.StatusBadRequest, "invalid payload", err)
        return
    }

    event, err := h.service.RecordAudit(r.Context(), id, service.AuditInput{
        EventType: payload.EventType,
        Actor:     payload.Actor,
        Payload:   payload.Payload,
    })
    if err != nil {
        status := http.StatusInternalServerError
        if errors.Is(err, service.ErrToolNotFound) {
            status = http.StatusNotFound
        }
        h.writeError(w, status, "record audit failed", err)
        return
    }

    h.writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if payload == nil {
        return
    }
    if err := json.NewEncoder(w).Encode(payload); err != nil {
        h.logger.Error("write json failed", zap.Error(err))
    }
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string, err error) {
    h.logger.Error(message, zap.Int("status", status), zap.Error(err))
    h.writeJSON(w, status, errorResponse{
        Error:   message,
        Details: err.Error(),
    })
}

type registerToolRequest struct {
    ID          string            `json:"id"`
    Owner       string            `json:"owner"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Inputs      map[string]string `json:"inputs"`
    Outputs     map[string]string `json:"outputs"`
    Policies    []policyRequest   `json:"policies"`
}

type policyRequest struct {
    ID           string         `json:"id"`
    Principal    string         `json:"principal"`
    AllowedScope []string       `json:"allowed_scope"`
    Conditions   map[string]any `json:"conditions"`
}

type auditRequest struct {
    EventType string          `json:"event_type"`
    Actor     string          `json:"actor"`
    Payload   map[string]any  `json:"payload"`
}

type errorResponse struct {
    Error   string `json:"error"`
    Details string `json:"details,omitempty"`
}

type toolPayload struct {
    ID          uuid.UUID         `json:"id"`
    Owner       string            `json:"owner"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Embedding   []float32         `json:"embedding"`
    Inputs      map[string]string `json:"inputs"`
    Outputs     map[string]string `json:"outputs"`
    CreatedAt   string            `json:"created_at"`
    UpdatedAt   string            `json:"updated_at"`
}

type policyPayload struct {
    ID           uuid.UUID        `json:"id"`
    Principal    string           `json:"principal"`
    AllowedScope []string         `json:"allowed_scope"`
    Conditions   map[string]any   `json:"conditions"`
    CreatedAt    string           `json:"created_at"`
}

type auditPayload struct {
    ID        int64           `json:"id"`
    EventType string          `json:"event_type"`
    Actor     string          `json:"actor"`
    Payload   map[string]any  `json:"payload"`
    CreatedAt string          `json:"created_at"`
}

type toolDetailsPayload struct {
    Tool     toolPayload     `json:"tool"`
    Policies []policyPayload `json:"policies"`
    Audits   []auditPayload  `json:"audits"`
}

func toToolPayload(tool domain.Tool) toolPayload {
    return toolPayload{
        ID:          tool.ID,
        Owner:       tool.Owner,
        Name:        tool.Name,
        Description: tool.Description,
        Embedding:   tool.Embedding,
        Inputs:      tool.Inputs,
        Outputs:     tool.Outputs,
        CreatedAt:   tool.CreatedAt.UTC().Format(time.RFC3339Nano),
        UpdatedAt:   tool.UpdatedAt.UTC().Format(time.RFC3339Nano),
    }
}

func toToolDetailsPayload(details domain.ToolDetails) toolDetailsPayload {
    policies := make([]policyPayload, 0, len(details.Policies))
    for _, policy := range details.Policies {
        policies = append(policies, policyPayload{
            ID:           policy.ID,
            Principal:    policy.Principal,
            AllowedScope: policy.AllowedScope,
            Conditions:   policy.Conditions,
            CreatedAt:    policy.CreatedAt.UTC().Format(time.RFC3339Nano),
        })
    }

    audits := make([]auditPayload, 0, len(details.Audits))
    for _, audit := range details.Audits {
        audits = append(audits, auditPayload{
            ID:        audit.ID,
            EventType: audit.EventType,
            Actor:     audit.Actor,
            Payload:   audit.Payload,
            CreatedAt: audit.CreatedAt.UTC().Format(time.RFC3339Nano),
        })
    }

    return toolDetailsPayload{
        Tool:     toToolPayload(details.Tool),
        Policies: policies,
        Audits:   audits,
    }
}
