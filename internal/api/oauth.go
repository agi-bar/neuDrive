package api

import (
	"context"
	"encoding/json"
	"errors"
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

var errOAuthConflictingClientAuth = errors.New("conflicting client authentication parameters")

// handleOAuthAuthorizeGet renders the consent page for GET /oauth/authorize.
func (s *Server) handleOAuthAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	responseType := r.URL.Query().Get("response_type")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

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

	if codeChallenge != "" && codeChallengeMethod != "S256" {
		auth.RenderConsentPage(w, auth.ConsentPageData{
			Error: "Invalid code_challenge_method. Only 'S256' is supported.",
		})
		return
	}

	// Look up the app. If client_id is a URL (MCP Client ID Metadata Document),
	// auto-register it on first use.
	app, err := s.OAuthService.GetAppByClientID(r.Context(), clientID)
	if err != nil && strings.HasPrefix(clientID, "https://") {
		// MCP Client ID Metadata Document: auto-register the client
		app, err = s.autoRegisterOAuthClient(r.Context(), clientID, redirectURI)
	}
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

	scopes, scope := effectiveOAuthScopes(app, scope)

	// Show login form if user is not authenticated (common for MCP connectors)
	_, isAuthed := userIDFromCtx(r.Context())

	auth.RenderConsentPage(w, auth.ConsentPageData{
		AppName:             app.Name,
		AppLogoURL:          app.LogoURL,
		Scopes:              scopes,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ShowLogin:           !isAuthed,
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
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")

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

	app, appErr := s.OAuthService.GetAppByClientID(r.Context(), clientID)
	scopes, scope := effectiveOAuthScopes(app, scope)
	appName := clientID
	appLogoURL := ""
	if app != nil {
		appName = app.Name
		appLogoURL = app.LogoURL
	}

	// If denied, redirect with error.
	if action != "approve" {
		q.Set("error", "access_denied")
		q.Set("error_description", "The user denied the authorization request.")
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	// Authenticate the user — try JWT from context first, then hidden token, then form login.
	userID, ok := userIDFromCtx(r.Context())

	// Try hidden _token field (auto-submitted by JS when user is already logged in)
	if !ok {
		if hiddenToken := r.FormValue("_token"); hiddenToken != "" {
			claims, err := auth.ValidateToken(hiddenToken, s.JWTSecret)
			if err == nil {
				userID = claims.UserID
				ok = true
			}
		}
	}

	if !ok {
		// No JWT — try form-based login (email + password in the form)
		email := r.FormValue("email")
		password := r.FormValue("password")
		if email == "" || password == "" {
			auth.RenderConsentPage(w, auth.ConsentPageData{
				Error:               "Please enter your email and password to authorize.",
				AppName:             appName,
				AppLogoURL:          appLogoURL,
				Scopes:              scopes,
				ClientID:            clientID,
				RedirectURI:         redirectURI,
				Scope:               scope,
				State:               state,
				CodeChallenge:       codeChallenge,
				CodeChallengeMethod: codeChallengeMethod,
				ShowLogin:           true,
			})
			return
		}
		// Authenticate via AuthService
		loginResp, err := s.AuthService.Login(r.Context(), models.LoginRequest{Email: email, Password: password}, r.UserAgent(), r.RemoteAddr)
		if err != nil {
			auth.RenderConsentPage(w, auth.ConsentPageData{
				Error:               "Login failed: " + err.Error(),
				AppName:             appName,
				AppLogoURL:          appLogoURL,
				Scopes:              scopes,
				ClientID:            clientID,
				RedirectURI:         redirectURI,
				Scope:               scope,
				State:               state,
				CodeChallenge:       codeChallenge,
				CodeChallengeMethod: codeChallengeMethod,
				ShowLogin:           true,
			})
			return
		}
		userID = loginResp.User.ID
		ok = true
	}
	_ = ok

	// Look up the app.
	if appErr != nil {
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

	// Generate the authorization code.
	code, err := s.OAuthService.Authorize(r.Context(), app.ID, userID, scopes, redirectURI, codeChallenge, codeChallengeMethod)
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
	req, err := parseOAuthTokenRequest(r)
	if err != nil {
		switch {
		case errors.Is(err, errOAuthConflictingClientAuth):
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Conflicting client authentication parameters.")
		case strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Failed to parse form data.")
		default:
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid request body.")
		}
		return
	}

	var (
		resp *models.OAuthTokenResponse
	)

	switch req.GrantType {
	case "authorization_code":
		if req.Code == "" || req.ClientID == "" || req.RedirectURI == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameters: code, client_id, redirect_uri.")
			return
		}
		resp, err = s.OAuthService.ExchangeCode(r.Context(), req.ClientID, req.ClientSecret, req.Code, req.RedirectURI, req.CodeVerifier)
	case "refresh_token":
		if req.RefreshToken == "" || req.ClientID == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameters: refresh_token, client_id.")
			return
		}
		resp, err = s.OAuthService.ExchangeRefreshToken(r.Context(), req.ClientID, req.ClientSecret, req.RefreshToken)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only 'authorization_code' and 'refresh_token' grant types are supported.")
		return
	}
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	// OAuth token response MUST be flat JSON (no envelope) per RFC 6749
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(resp)
}

func parseOAuthTokenRequest(r *http.Request) (models.OAuthTokenRequest, error) {
	contentType := r.Header.Get("Content-Type")

	var req models.OAuthTokenRequest

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			return req, err
		}
		req.GrantType = r.FormValue("grant_type")
		req.Code = r.FormValue("code")
		req.ClientID = r.FormValue("client_id")
		req.ClientSecret = r.FormValue("client_secret")
		req.RedirectURI = r.FormValue("redirect_uri")
		req.CodeVerifier = r.FormValue("code_verifier")
		req.RefreshToken = r.FormValue("refresh_token")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, err
		}
	}

	if basicID, basicSecret, ok := r.BasicAuth(); ok {
		if req.ClientID != "" && req.ClientID != basicID {
			return req, errOAuthConflictingClientAuth
		}
		if req.ClientSecret != "" && req.ClientSecret != basicSecret {
			return req, errOAuthConflictingClientAuth
		}
		req.ClientID = basicID
		req.ClientSecret = basicSecret
	}

	return req, nil
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

	respondOK(w, resp)
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

// handleOAuthAuthorizeInfo returns app info for the SPA consent page.
// GET /api/oauth/authorize-info?client_id=...&redirect_uri=...&scope=...&state=...&response_type=code
func (s *Server) handleOAuthAuthorizeInfo(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	scope := r.URL.Query().Get("scope")
	responseType := r.URL.Query().Get("response_type")

	if responseType != "code" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid response_type. Only 'code' is supported.")
		return
	}

	if clientID == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "Missing required parameter: client_id")
		return
	}

	// Look up the app. Auto-register URL-based client_ids.
	app, err := s.OAuthService.GetAppByClientID(r.Context(), clientID)
	if err != nil && strings.HasPrefix(clientID, "https://") {
		app, err = s.autoRegisterOAuthClient(r.Context(), clientID, redirectURI)
	}
	if err != nil {
		respondError(w, http.StatusNotFound, ErrCodeNotFound, "Unknown application. The client_id is not registered.")
		return
	}

	if !app.IsActive {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "This application has been deactivated.")
		return
	}

	if redirectURI == "" {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "Missing required parameter: redirect_uri")
		return
	}
	if !s.OAuthService.ValidateRedirectURI(app, redirectURI) {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid redirect_uri.")
		return
	}

	scopes, scope := effectiveOAuthScopes(app, scope)
	scopeLabels := make([]map[string]string, 0, len(scopes))
	for _, sc := range scopes {
		scopeLabels = append(scopeLabels, map[string]string{
			"scope": sc,
			"label": auth.ScopeLabel(sc),
		})
	}

	respondOK(w, map[string]interface{}{
		"app_name":     app.Name,
		"app_logo":     app.LogoURL,
		"scopes":       scopeLabels,
		"client_id":    clientID,
		"redirect_uri": redirectURI,
		"scope":        scope,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// autoRegisterOAuthClient auto-registers an MCP client using Client ID Metadata Document.
// When client_id is an HTTPS URL, we register it as an OAuth app with that URL as the client_id.
func (s *Server) autoRegisterOAuthClient(ctx context.Context, clientIDURL, redirectURI string) (*models.OAuthApp, error) {
	parsed, _ := url.Parse(clientIDURL)
	appName := "MCP Client"
	if parsed != nil {
		appName = parsed.Host
	}

	redirectURIs := []string{}
	if redirectURI != "" {
		redirectURIs = append(redirectURIs, redirectURI)
	}
	redirectURIs = append(redirectURIs,
		"https://claude.ai/api/mcp/auth_callback",
		"https://claude.com/api/mcp/auth_callback",
	)

	// Use the demo user as the system owner for auto-registered apps.
	// In production this should be a dedicated system user.
	systemUserID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")

	return s.OAuthService.RegisterAppWithClientID(ctx, systemUserID,
		clientIDURL, appName, redirectURIs, []string{"admin"})
}

func effectiveOAuthScopes(app *models.OAuthApp, requestedScope string) ([]string, string) {
	scopes := auth.SplitScopes(requestedScope)
	if len(scopes) == 0 && app != nil {
		scopes = append([]string(nil), app.Scopes...)
	}
	if len(scopes) == 0 {
		return nil, ""
	}
	return scopes, strings.Join(scopes, " ")
}

// writeOAuthError writes an OAuth 2.0 compliant error response.
func writeOAuthError(w http.ResponseWriter, status int, errCode, description string) {
	writeJSON(w, status, map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}
