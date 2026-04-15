package api

import "net/http"

func (s *Server) handleLocalGitMirrorGet(w http.ResponseWriter, r *http.Request) {
	if !s.systemSettingsEnabled() {
		respondForbidden(w, "system settings are disabled")
		return
	}
	s.handleGitMirrorGet(w, r)
}

func (s *Server) handleLocalGitMirrorUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.systemSettingsEnabled() {
		respondForbidden(w, "system settings are disabled")
		return
	}
	s.handleGitMirrorUpdate(w, r)
}

func (s *Server) handleLocalGitMirrorGitHubTest(w http.ResponseWriter, r *http.Request) {
	if !s.systemSettingsEnabled() {
		respondForbidden(w, "system settings are disabled")
		return
	}
	s.handleGitMirrorGitHubTest(w, r)
}
