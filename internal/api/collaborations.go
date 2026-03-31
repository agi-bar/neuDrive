package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Collaboration API handlers
// ---------------------------------------------------------------------------

type createCollaborationRequest struct {
	GuestSlug    string   `json:"guest_slug"`
	SharedPaths  []string `json:"shared_paths"`
	Permissions  string   `json:"permissions"`
	ExpiresInDays *int    `json:"expires_in_days,omitempty"`
}

func (s *Server) handleListCollaborations(w http.ResponseWriter, r *http.Request) {
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

	respondCreated(w, collab)
}

func (s *Server) handleRevokeCollaboration(w http.ResponseWriter, r *http.Request) {
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
	if !s.agentCheckAuth(w, r, 2, "") { // L2 Collaborate
		return
	}

	guestUserID, _ := userIDFromCtx(r.Context())

	ownerSlug := chi.URLParam(r, "owner_slug")
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}

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

	// Return the shared file tree content (stub — real implementation would read from file_tree).
	respondOK(w, map[string]interface{}{
		"owner":    ownerSlug,
		"path":     path,
		"children": []interface{}{},
	})
}
