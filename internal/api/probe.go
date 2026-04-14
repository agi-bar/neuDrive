package api

import "net/http"

// handleTestPost provides a minimal unauthenticated POST probe endpoint.
// Restricted sandboxes can use it to verify whether outbound POST requests to
// the neuDrive host are allowed before attempting a direct multipart upload.
func (s *Server) handleTestPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, ErrCodeBadRequest, "method not allowed")
		return
	}
	respondOK(w, map[string]interface{}{
		"reachable": true,
		"method":    http.MethodPost,
		"path":      "/test/post",
	})
}
