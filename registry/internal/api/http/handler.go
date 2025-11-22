package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ashrafalaodat/smcp/registry/internal/domain"
	"github.com/ashrafalaodat/smcp/registry/internal/persistence"
	"github.com/ashrafalaodat/smcp/registry/internal/service"
)

// Handler exposes HTTP endpoints.
type Handler struct {
	svc    *service.Service
	logger *zap.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *service.Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes wires HTTP routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		r.Post("/tools", h.handleRegisterTool)
		r.Get("/tools", h.handleListTools)
		r.Get("/tools/{owner}/{name}", h.handleGetTool)
		r.Delete("/tools/{owner}/{name}", h.handleDeleteTool)
		r.Post("/tools/search", h.handleSearchTools)
	})
}

type registerToolRequest struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Visibility  string `json:"visibility"`
	Description string `json:"description"`
}

type searchToolsRequest struct {
	Owner       string `json:"owner"`
	Visibility  string `json:"visibility"`
	Limit       int    `json:"limit"`
	Description string `json:"description"`
}

type toolResponse struct {
	Owner      string    `json:"owner"`
	Name       string    `json:"name"`
	Visibility string    `json:"visibility"`
	Embedding  []float32 `json:"embedding"`
}

type searchResponse struct {
	Tool  toolResponse `json:"tool"`
	Score float32      `json:"score"`
}

func (h *Handler) handleRegisterTool(w http.ResponseWriter, r *http.Request) {
	var req registerToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}

	record, err := h.svc.RegisterTool(r.Context(), service.RegisterToolInput{
		Owner:       req.Owner,
		Name:        req.Name,
		Visibility:  req.Visibility,
		Description: req.Description,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, toToolResponse(record))
}

func (h *Handler) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := h.svc.ListTools(r.Context(), service.ListToolsInput{
		Owner:      r.URL.Query().Get("owner"),
		Visibility: r.URL.Query().Get("visibility"),
	})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "list tools failed", err)
		return
	}

	resp := make([]toolResponse, 0, len(tools))
	for _, t := range tools {
		resp = append(resp, toToolResponse(t))
	}
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleGetTool(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "name")

	tool, err := h.svc.GetTool(r.Context(), owner, name)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, toToolResponse(tool))
}

func (h *Handler) handleDeleteTool(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "name")

	if err := h.svc.DeleteTool(r.Context(), owner, name); err != nil {
		h.handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSearchTools(w http.ResponseWriter, r *http.Request) {
	var req searchToolsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}

	results, err := h.svc.SearchTools(r.Context(), service.SearchToolsInput{
		Owner:       req.Owner,
		Visibility:  req.Visibility,
		Limit:       req.Limit,
		Description: req.Description,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	resp := make([]searchResponse, 0, len(results))
	for _, result := range results {
		resp = append(resp, searchResponse{
			Tool:  toToolResponse(result.Tool),
			Score: result.Score,
		})
	}
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "resource not found", err)
	case errors.Is(err, service.ErrVectorizerUnavailable):
		h.writeError(w, http.StatusServiceUnavailable, "vectorizer unavailable", err)
	case service.IsValidationError(err):
		h.writeError(w, http.StatusBadRequest, err.Error(), err)
	default:
		h.writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.logger.Error("write response failed", zap.Error(err))
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string, err error) {
	h.logger.Warn(message, zap.Error(err))
	type errorResponse struct {
		Error string `json:"error"`
	}
	h.writeJSON(w, status, errorResponse{Error: message})
}

func toToolResponse(tool domain.ToolRecord) toolResponse {
	return toolResponse{
		Owner:      tool.Owner,
		Name:       tool.Name,
		Visibility: tool.Visibility,
		Embedding:  tool.Embedding,
	}
}
