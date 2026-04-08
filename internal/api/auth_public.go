package api

import "net/http"

func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if s.AuthHandler == nil {
		respondNotConfigured(w, "auth service")
		return
	}
	s.AuthHandler.HandleRegister(w, r)
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.AuthHandler == nil {
		respondNotConfigured(w, "auth service")
		return
	}
	s.AuthHandler.HandleLogin(w, r)
}

func (s *Server) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	if s.AuthHandler == nil {
		respondNotConfigured(w, "auth service")
		return
	}
	s.AuthHandler.HandleRefresh(w, r)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if s.AuthHandler == nil {
		respondNotConfigured(w, "auth service")
		return
	}
	s.AuthHandler.HandleLogout(w, r)
}

func (s *Server) handleAuthGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if s.AuthHandler == nil {
		respondNotConfigured(w, "auth service")
		return
	}
	s.AuthHandler.HandleGitHubCallback(w, r)
}

func (s *Server) handleAuthDevToken(w http.ResponseWriter, r *http.Request) {
	if s.AuthHandler == nil {
		respondNotConfigured(w, "auth service")
		return
	}
	s.AuthHandler.HandleDevToken(w, r)
}
