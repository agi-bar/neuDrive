package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// OAuth 2.0 Provider Endpoints
// ---------------------------------------------------------------------------

// handleOAuthAuthorizeGet renders the consent page for GET /oauth/authorize.
func (s *Server) handleOAuthAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	responseType := r.URL.Query().Get("response_type")

	// Validate response_type.
	if responseType != "code" {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Invalid response_type. Only 'code' is supported.",
		})
		return
	}

	if clientID == "" {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Missing required parameter: client_id",
		})
		return
	}

	// Look up the app.
	app, err := s.OAuthService.GetAppByClientID(r.Context(), clientID)
	if err != nil {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Unknown application. The client_id is not registered.",
		})
		return
	}

	if !app.IsActive {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "This application has been deactivated.",
		})
		return
	}

	// Validate redirect_uri.
	if redirectURI == "" {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Missing required parameter: redirect_uri",
		})
		return
	}
	if !s.OAuthService.ValidateRedirectURI(app, redirectURI) {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Invalid redirect_uri. It is not registered for this application.",
		})
		return
	}

	scopes := auth.SplitScopes(scope)

	auth.RenderConsentPage(w, auth.ConsentPageData{
		AppName:     app.Name,
		AppLogoURL:  app.LogoURL,
		Scopes:      scopes,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scope:       scope,
		State:       state,
	})
}

// handleOAuthAuthorizePost processes the user's approve/deny action.
func (s *Server) handleOAuthAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Invalid form data.",
		})
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scope := r.FormValue("scope")
	state := r.FormValue("state")
	action := r.FormValue("action")

	// Build the redirect URL.
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Invalid redirect_uri.",
		})
		return
	}

	q := redirectURL.Query()
	if state != "" {
		q.Set("state", state)
	}

	// If denied, redirect with error.
	if action != "approve" {
		q.Set("error", "access_denied")
		q.Set("error_description", "The user denied the authorization request.")
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	// Get the authenticated user.
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "You must be logged in to authorize applications. Please log in first.",
		})
		return
	}

	// Look up the app.
	app, err := s.OAuthService.GetAppByClientID(r.Context(), clientID)
	if err != nil {
		q.Set("error", "invalid_request")
		q.Set("error_description", "Unknown application.")
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	if !s.OAuthService.ValidateRedirectURI(app, redirectURI) {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Invalid redirect_uri.",
		})
		return
	}

	scopes := auth.SplitScopes(scope)

	// Generate the authorization code.
	code, err := s.OAuthService.Authorize(r.Context(), app.ID, userID, scopes, redirectURI)
	if err != nil {
		q.Set("error", "server_error")
		q.Set("error_description", "Failed to generate authorization code.")
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	q.Set("code", code)
	redirectURL.RawQuery = q.Encode()
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// handleOAuthToken handles POST /oauth/token (code exchange).
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")

	var req models.OAuthTokenRequest

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Failed to parse form data.")
			return
		}
		req.GrantType = r.FormValue("grant_type")
		req.Code = r.FormValue("code")
		req.ClientID = r.FormValue("client_id")
		req.ClientSecret = r.FormValue("client_secret")
		req.RedirectURI = r.FormValue("redirect_uri")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid request body.")
			return
		}
	}

	if req.GrantType != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only 'authorization_code' grant type is supported.")
		return
	}

	if req.Code == "" || req.ClientID == "" || req.ClientSecret == "" || req.RedirectURI == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameters: code, client_id, client_secret, redirect_uri.")
		return
	}

	resp, err := s.OAuthService.ExchangeCode(r.Context(), req.ClientID, req.ClientSecret, req.Code, req.RedirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, resp)
}

// handleOAuthUserInfo handles GET /oauth/userinfo.
func (s *Server) handleOAuthUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "Missing or invalid access token.")
		return
	}

	user, err := s.UserService.GetByID(r.Context(), userID)
	if err != nil {
		writeOAuthError(w, http.StatusNotFound, "invalid_token", "User not found.")
		return
	}

	resp := models.OAuthUserInfoResponse{
		Sub:       user.ID.String(),
		Name:      user.DisplayName,
		Slug:      user.Slug,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
		Timezone:  user.Timezone,
		Language:  user.Language,
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// OAuth App Management Endpoints (authenticated)
// ---------------------------------------------------------------------------

// handleListOAuthApps handles GET /api/oauth/apps.
func (s *Server) handleListOAuthApps(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	apps, err := s.OAuthService.ListApps(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	responses := make([]models.OAuthAppResponse, 0, len(apps))
	for i := range apps {
		responses = append(responses, apps[i].ToResponse())
	}

	respondOK(w, map[string]interface{}{
		"apps": responses,
	})
}

// handleRegisterOAuthApp handles POST /api/oauth/apps.
func (s *Server) handleRegisterOAuthApp(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req models.RegisterOAuthAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, "name", "name is required")
		return
	}
	if len(req.RedirectURIs) == 0 {
		respondValidationError(w, "redirect_uris", "at least one redirect_uri is required")
		return
	}

	resp, err := s.OAuthService.RegisterApp(r.Context(), userID, req.Name, req.RedirectURIs, req.Scopes, req.Description, req.LogoURL)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, err.Error())
		return
	}

	respondCreated(w, resp)
}

// handleDeleteOAuthApp handles DELETE /api/oauth/apps/{id}.
func (s *Server) handleDeleteOAuthApp(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	appID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid app ID")
		return
	}

	if err := s.OAuthService.DeleteApp(r.Context(), userID, appID); err != nil {
		respondNotFound(w, "oauth app")
		return
	}

	respondOK(w, map[string]string{"status": "deleted"})
}

// handleListOAuthGrants handles GET /api/oauth/grants.
func (s *Server) handleListOAuthGrants(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	grants, err := s.OAuthService.ListGrants(r.Context(), userID)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"grants": grants,
	})
}

// handleRevokeOAuthGrant handles DELETE /api/oauth/grants/{id}.
func (s *Server) handleRevokeOAuthGrant(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	idStr := chi.URLParam(r, "id")
	grantID, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid grant ID")
		return
	}

	if err := s.OAuthService.RevokeGrant(r.Context(), userID, grantID); err != nil {
		respondNotFound(w, "oauth grant")
		return
	}

	respondOK(w, map[string]string{"status": "revoked"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeOAuthError writes an OAuth 2.0 compliant error response.
func writeOAuthError(w http.ResponseWriter, status int, errCode, description string) {
	writeJSON(w, status, map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}
