package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Collaboration API handlers
// ---------------------------------------------------------------------------

type createCollaborationRequest struct {
	GuestSlug     string   `json:"guest_slug"`
	SharedPaths   []string `json:"shared_paths"`
	Permissions   string   `json:"permissions"`
	ExpiresInDays *int     `json:"expires_in_days,omitempty"`
}

func (s *Server) handleListCollaborations(w http.ResponseWriter, r *http.Request) {
	if s.CollaborationService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "collaboration service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	owned, err := s.CollaborationService.ListOwned(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	shared, err := s.CollaborationService.ListShared(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"owned":  owned,
		"shared": shared,
	})
}

func (s *Server) handleCreateCollaboration(w http.ResponseWriter, r *http.Request) {
	if s.CollaborationService == nil || s.UserService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "collaboration service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req createCollaborationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.GuestSlug == "" {
		respondValidationError(w, "guest_slug", "guest_slug is required")
		return
	}
	if len(req.SharedPaths) == 0 {
		respondValidationError(w, "shared_paths", "shared_paths must not be empty")
		return
	}
	for i, sharedPath := range req.SharedPaths {
		req.SharedPaths[i] = hubpath.NormalizePublic(sharedPath)
	}

	// Look up guest user by slug.
	guest, err := s.UserService.GetBySlug(r.Context(), req.GuestSlug)
	if err != nil {
		respondNotFound(w, "guest user")
		return
	}

	collab, err := s.CollaborationService.Create(
		r.Context(), userID, guest.ID,
		req.SharedPaths, req.Permissions, req.ExpiresInDays,
	)
	if err != nil {
		respondError(w, http.StatusConflict, ErrCodeConflict, err.Error())
		return
	}

	if s.WebhookService != nil {
		go s.WebhookService.Trigger(context.Background(), userID, models.EventCollabNew, map[string]interface{}{
			"collaboration_id": collab.ID.String(),
			"guest_user_id":    collab.GuestUserID.String(),
			"shared_paths":     collab.SharedPaths,
			"permissions":      collab.Permissions,
		})
	}

	respondCreated(w, collab)
}

func (s *Server) handleRevokeCollaboration(w http.ResponseWriter, r *http.Request) {
	if s.CollaborationService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "collaboration service not configured")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	collabIDStr := chi.URLParam(r, "id")
	collabID, err := uuid.Parse(collabIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid collaboration id")
		return
	}

	if err := s.CollaborationService.Revoke(r.Context(), collabID, userID); err != nil {
		respondNotFound(w, "collaboration")
		return
	}

	respondOK(w, map[string]string{"status": "revoked"})
}

// ---------------------------------------------------------------------------
// Agent API: cross-user shared tree access
// ---------------------------------------------------------------------------

func (s *Server) handleAgentSharedTree(w http.ResponseWriter, r *http.Request) {
	if s.CollaborationService == nil || s.UserService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "collaboration service not configured")
		return
	}
	if !s.agentCheckAuth(w, r, 2, "") { // L2 Collaborate
		return
	}

	guestUserID, _ := userIDFromCtx(r.Context())

	ownerSlug := chi.URLParam(r, "owner_slug")
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}
	path = hubpath.NormalizePublic(path)

	// Resolve the owner user.
	owner, err := s.UserService.GetBySlug(r.Context(), ownerSlug)
	if err != nil {
		respondNotFound(w, "owner user")
		return
	}

	// Check that the guest has access to this path.
	canAccess, err := s.CollaborationService.CanAccess(r.Context(), guestUserID, owner.ID, path)
	if err != nil || !canAccess {
		respondForbidden(w, "you do not have access to this path")
		return
	}

	node, err := s.readOrListTreePath(r.Context(), owner.ID, models.TrustLevelCollaborate, path)
	if err != nil {
		respondNotFound(w, "file")
		return
	}

	body := map[string]interface{}{
		"owner": ownerSlug,
		"path":  node.Path,
		"node":  node,
	}
	if node.IsDir {
		body["children"] = node.Children
	} else {
		body["content"] = node.Content
		body["content_type"] = node.MimeType
	}
	respondOK(w, body)
}
