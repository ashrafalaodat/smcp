package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

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
		r.Post("/servers", h.handleRegisterServer)
		r.Put("/servers/{id}/heartbeat", h.handleHeartbeat)
		r.Get("/servers", h.handleListServers)
	})
}

func (h *Handler) handleRegisterServer(w http.ResponseWriter, r *http.Request) {
	var payload registerServerRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}

	var serverID *uuid.UUID
	if payload.ID != "" {
		id, err := uuid.Parse(payload.ID)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid id", err)
			return
		}
		serverID = &id
	}

	input := service.RegisterServerInput{
		ID:         serverID,
		Name:       payload.Name,
		Endpoint:   payload.Endpoint,
		Version:    payload.Version,
		Region:     payload.Region,
		AuthMethod: payload.AuthMethod,
		Metadata:   payload.Metadata,
	}

	for _, cap := range payload.Capabilities {
		var capabilityID *uuid.UUID
		if cap.ID != "" {
			id, err := uuid.Parse(cap.ID)
			if err != nil {
				h.writeError(w, http.StatusBadRequest, "invalid capability id", err)
				return
			}
			capabilityID = &id
		}
		input.Capabilities = append(input.Capabilities, service.CapabilityInput{
			ID:     capabilityID,
			Type:   cap.Type,
			Name:   cap.Name,
			Schema: cap.Schema,
			Tags:   cap.Tags,
		})
	}

	for _, pol := range payload.Policies {
		var policyID *uuid.UUID
		if pol.ID != "" {
			id, err := uuid.Parse(pol.ID)
			if err != nil {
				h.writeError(w, http.StatusBadRequest, "invalid policy id", err)
				return
			}
			policyID = &id
		}
		input.Policies = append(input.Policies, service.PolicyInput{
			ID:           policyID,
			Principal:    pol.Principal,
			AllowedTools: pol.AllowedTools,
			Conditions:   pol.Conditions,
		})
	}

	snapshot, err := h.service.RegisterServer(r.Context(), input)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "register server failed", err)
		return
	}

	h.writeJSON(w, http.StatusCreated, toServerSnapshotPayload(snapshot))
}

func (h *Handler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	serverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid server id", err)
		return
	}

	var payload heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}

	snapshot, err := h.service.ReportHeartbeat(r.Context(), serverID, service.HeartbeatInput{
		Status:    payload.Status,
		LatencyMS: payload.LatencyMS,
		ErrorRate: payload.ErrorRate,
		Details:   payload.Details,
	})
	if err != nil {
		if errors.Is(err, service.ErrServerNotFound) {
			h.writeError(w, http.StatusNotFound, "server not found", err)
			return
		}
		h.writeError(w, http.StatusInternalServerError, "heartbeat failed", err)
		return
	}

	h.writeJSON(w, http.StatusOK, toServerSnapshotPayload(snapshot))
}

func (h *Handler) handleListServers(w http.ResponseWriter, r *http.Request) {
	input := service.ListServersInput{
		Region: r.URL.Query().Get("region"),
		Status: r.URL.Query().Get("status"),
		Tag:    r.URL.Query().Get("tag"),
	}

	snapshots, err := h.service.ListServers(r.Context(), input)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "list servers failed", err)
		return
	}

	response := make([]serverSnapshotPayload, 0, len(snapshots))
	for _, snapshot := range snapshots {
		response = append(response, toServerSnapshotPayload(snapshot))
	}
	h.writeJSON(w, http.StatusOK, response)
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

type registerServerRequest struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Endpoint     string                    `json:"endpoint"`
	Version      string                    `json:"version"`
	Region       string                    `json:"region"`
	AuthMethod   string                    `json:"auth_method"`
	Metadata     map[string]any            `json:"metadata"`
	Capabilities []capabilityRequest       `json:"capabilities"`
	Policies     []policyRequest           `json:"policies"`
}

type capabilityRequest struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Tags   []string       `json:"tags"`
}

type policyRequest struct {
	ID           string         `json:"id"`
	Principal    string         `json:"principal"`
	AllowedTools []string       `json:"allowed_tools"`
	Conditions   map[string]any `json:"conditions"`
}

type heartbeatRequest struct {
	Status    string         `json:"status"`
	LatencyMS int            `json:"latency_ms"`
	ErrorRate float64        `json:"error_rate"`
	Details   map[string]any `json:"details"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type serverSnapshotPayload struct {
	Server       domain.Server      `json:"server"`
	Capabilities []domain.Capability `json:"capabilities,omitempty"`
	Policies     []domain.Policy     `json:"policies,omitempty"`
	Health       *domain.Health      `json:"health,omitempty"`
}

func toServerSnapshotPayload(snapshot domain.ServerSnapshot) serverSnapshotPayload {
	return serverSnapshotPayload{
		Server:       snapshot.Server,
		Capabilities: snapshot.Capabilities,
		Policies:     snapshot.Policies,
		Health:       snapshot.Health,
	}
}
