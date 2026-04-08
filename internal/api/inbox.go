package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Message struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"created_at"`
}

type SendMessageRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Server) handleInboxList(w http.ResponseWriter, r *http.Request) {
	if s.InboxService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "inbox service not configured")
		return
	}
	role := chi.URLParam(r, "role")

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	status := r.URL.Query().Get("status")

	messages, err := s.InboxService.GetMessages(r.Context(), userID, role, status)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"role":     role,
		"messages": messages,
	})
}

func (s *Server) handleInboxSend(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.To == "" || req.Body == "" {
		respondValidationError(w, "to,body", "to and body are required")
		return
	}
	if s.InboxService == nil {
		respondNotConfigured(w, "inbox service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	msg := models.InboxMessage{
		FromAddress: "assistant@" + userID.String(),
		ToAddress:   req.To,
		Subject:     req.Subject,
		Body:        req.Body,
		Priority:    "normal",
	}

	sent, err := s.InboxService.Send(r.Context(), userID, msg)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondCreated(w, sent)
}

func (s *Server) handleInboxArchive(w http.ResponseWriter, r *http.Request) {
	if s.InboxService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "inbox service not configured")
		return
	}
	idStr := chi.URLParam(r, "id")
	msgID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid message ID")
		return
	}

	if _, ok := userIDFromCtx(r.Context()); !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.InboxService.Archive(r.Context(), msgID); err != nil {
		respondNotFound(w, "message")
		return
	}

	respondOK(w, map[string]string{"status": "archived", "id": idStr})
}

func (s *Server) handleInboxSearch(w http.ResponseWriter, r *http.Request) {
	if s.InboxService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "inbox service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		respondValidationError(w, "q", "search query is required")
		return
	}

	scope := r.URL.Query().Get("scope")

	messages, err := s.InboxService.Search(r.Context(), userID, query, scope)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"query":   query,
		"results": messages,
	})
}
