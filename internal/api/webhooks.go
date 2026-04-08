package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// --- Request types ---

type registerWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// --- Handlers ---

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	if s.WebhookService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "webhook service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	webhooks, err := s.WebhookService.List(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	if webhooks == nil {
		webhooks = []models.Webhook{}
	}

	respondOK(w, map[string]interface{}{
		"webhooks": webhooks,
	})
}

func (s *Server) handleRegisterWebhook(w http.ResponseWriter, r *http.Request) {
	if s.WebhookService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "webhook service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req registerWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.URL == "" {
		respondValidationError(w, "url", "url is required")
		return
	}
	if len(req.Events) == 0 {
		respondValidationError(w, "events", "at least one event type is required")
		return
	}

	// Validate event types.
	for _, ev := range req.Events {
		if !models.ValidWebhookEvents[ev] {
			respondValidationError(w, "events", "unknown event type: "+ev)
			return
		}
	}

	wh, secret, err := s.WebhookService.Register(r.Context(), userID, req.URL, req.Events)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreated(w, map[string]interface{}{
		"webhook": wh,
		"secret":  secret,
	})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if s.WebhookService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "webhook service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	webhookID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid webhook id")
		return
	}

	if err := s.WebhookService.Delete(r.Context(), webhookID, userID); err != nil {
		respondNotFound(w, "webhook")
		return
	}

	respondOK(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	if s.WebhookService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "webhook service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	webhookID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid webhook id")
		return
	}

	if err := s.WebhookService.Test(r.Context(), webhookID, userID); err != nil {
		respondNotFound(w, "webhook")
		return
	}

	respondOK(w, map[string]string{"status": "test event sent"})
}
