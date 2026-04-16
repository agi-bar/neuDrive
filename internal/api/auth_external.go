package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	if s.ExternalAuthService == nil {
		respondOK(w, []models.AuthProvider{})
		return
	}
	respondOK(w, s.ExternalAuthService.ListProviders())
}

func (s *Server) handleAuthProviderStart(w http.ResponseWriter, r *http.Request) {
	if s.ExternalAuthService == nil {
		respondNotConfigured(w, "external auth service")
		return
	}
	providerKey := strings.TrimSpace(chi.URLParam(r, "provider"))
	var req models.StartAuthProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	callbackURL := s.baseURL(r) + "/api/auth/providers/" + url.PathEscape(providerKey) + "/callback"
	redirectURL := s.normalizeAuthRedirectURL(r, req.RedirectURL)
	resp, err := s.ExternalAuthService.Start(r.Context(), providerKey, callbackURL, redirectURL, req.Action)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleAuthProviderCallback(w http.ResponseWriter, r *http.Request) {
	if s.ExternalAuthService == nil {
		http.Redirect(w, r, s.authErrorRedirect("", "external auth service is not configured"), http.StatusFound)
		return
	}
	providerKey := strings.TrimSpace(chi.URLParam(r, "provider"))
	callbackURL := s.baseURL(r) + "/api/auth/providers/" + url.PathEscape(providerKey) + "/callback"
	result, err := s.ExternalAuthService.Complete(
		r.Context(),
		providerKey,
		r.URL.Query().Get("state"),
		r.URL.Query().Get("code"),
		r.URL.Query().Get("error"),
		r.URL.Query().Get("error_description"),
		callbackURL,
		r.UserAgent(),
		r.RemoteAddr,
	)
	if err != nil {
		redirectURL := ""
		if result != nil {
			redirectURL = result.RedirectURL
		}
		http.Redirect(w, r, s.authErrorRedirect(redirectURL, err.Error()), http.StatusFound)
		return
	}
	if result == nil || result.Auth == nil {
		http.Redirect(w, r, s.authErrorRedirect("", "authentication did not complete"), http.StatusFound)
		return
	}
	http.Redirect(w, r, s.authSuccessRedirect(result.RedirectURL, result.Auth), http.StatusFound)
}

func (s *Server) normalizeAuthRedirectURL(r *http.Request, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	target, err := url.Parse(raw)
	if err != nil || !target.IsAbs() {
		return "/"
	}
	base, err := url.Parse(s.baseURL(r))
	if err != nil {
		return "/"
	}
	if strings.EqualFold(target.Scheme, base.Scheme) && strings.EqualFold(target.Host, base.Host) {
		return target.String()
	}
	return "/"
}

func (s *Server) authSuccessRedirect(target string, authResp *models.AuthResponse) string {
	redirectURL := strings.TrimSpace(target)
	if redirectURL == "" {
		redirectURL = "/"
	}
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		parsed = &url.URL{Path: "/"}
	}
	query := parsed.Query()
	query.Set("auth_token", authResp.AccessToken)
	query.Set("auth_refresh", authResp.RefreshToken)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (s *Server) authErrorRedirect(target, message string) string {
	parsed, err := url.Parse("/login")
	if err != nil {
		return "/login"
	}
	query := parsed.Query()
	if redirect := strings.TrimSpace(target); redirect != "" && redirect != "/login" {
		query.Set("redirect", redirect)
	}
	if msg := strings.TrimSpace(message); msg != "" {
		query.Set("error", msg)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
