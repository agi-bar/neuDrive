package api

import "net/http"

func (s *Server) handleSkillsList(w http.ResponseWriter, r *http.Request) {
	if s.FileTreeService == nil {
		respondError(w, http.StatusNotImplemented, ErrCodeUnsupported, "file tree service not configured")
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	skills, err := s.listSkills(r.Context(), userID, trustLevelFromCtx(r.Context()))
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"skills": skills,
	})
}
