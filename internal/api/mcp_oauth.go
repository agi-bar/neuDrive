package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type oauthDynamicRegisterRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	Scope                   string   `json:"scope"`
	ClientID                string   `json:"client_id"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	ApplicationType         string   `json:"application_type"`
	ResponseTypes           []string `json:"response_types"`
}

// ---------------------------------------------------------------------------
// OAuth 2.0 Protected Resource Metadata (RFC 9728) for MCP
//
// Claude.ai Custom Connectors require OAuth discovery:
// 1. POST /mcp → 401 with WWW-Authenticate header pointing to resource_metadata
// 2. GET /.well-known/oauth-protected-resource → resource metadata JSON
// 3. GET /.well-known/oauth-authorization-server → authorization server metadata
// 4. Claude.ai performs OAuth flow → gets access token → retries POST /mcp
// ---------------------------------------------------------------------------

// handleProtectedResourceMetadata serves RFC 9728 Protected Resource Metadata.
// This tells Claude.ai where to find the authorization server.
func (s *Server) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	baseURL := s.baseURL(r)

	metadata := map[string]interface{}{
		"resource":              baseURL + "/mcp",
		"authorization_servers": []string{baseURL},
		"scopes_supported": []string{
			"read:profile", "write:profile",
			"read:memory", "write:memory",
			"read:skills", "write:skills",
			"read:vault", "read:vault.auth", "write:vault",
			"read:devices", "call:devices",
			"read:inbox", "write:inbox",
			"read:projects", "write:projects",
			"read:tree", "write:tree",
			"search", "admin", "offline_access",
		},
		"bearer_methods_supported": []string{"header"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(metadata)
}

// handleAuthorizationServerMetadata serves OAuth 2.0 Authorization Server Metadata (RFC 8414).
func (s *Server) handleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	baseURL := s.baseURL(r)

	metadata := map[string]interface{}{
		"issuer":                 baseURL,
		"authorization_endpoint": baseURL + "/oauth/authorize",
		"token_endpoint":         baseURL + "/oauth/token",
		"registration_endpoint":  baseURL + "/oauth/register",
		"scopes_supported": []string{
			"read:profile", "write:profile",
			"read:memory", "write:memory",
			"read:skills", "write:skills",
			"read:vault", "write:vault",
			"read:devices", "call:devices",
			"read:inbox", "write:inbox",
			"read:projects", "write:projects",
			"read:tree", "write:tree",
			"search", "admin", "offline_access",
		},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"client_id_metadata_document_supported": true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(metadata)
}

// handleOAuthDynamicRegister handles Dynamic Client Registration (RFC 7591).
// Claude.ai may use this to register itself as an OAuth client.
func (s *Server) handleOAuthDynamicRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	req, err := parseOAuthDynamicRegisterRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_request"})
		return
	}

	if req.ClientName == "" {
		req.ClientName = "Claude"
	}
	if len(req.RedirectURIs) == 0 {
		req.RedirectURIs = []string{"https://claude.ai/api/mcp/auth_callback"}
	}

	// Use existing OAuth service to register the app
	userID := getSystemUserID(s)
	resp, err := s.OAuthService.RegisterApp(r.Context(), userID,
		req.ClientName, req.RedirectURIs, strings.Split(req.Scope, " "),
		"Dynamic client registration via MCP", "")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "server_error", "error_description": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":                  resp.ClientID,
		"client_secret":              resp.ClientSecret,
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                req.GrantTypes,
		"token_endpoint_auth_method": "client_secret_post",
	})
}

func parseOAuthDynamicRegisterRequest(r *http.Request) (oauthDynamicRegisterRequest, error) {
	var req oauthDynamicRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func (s *Server) baseURL(r *http.Request) string {
	if s != nil && s.Config != nil && s.Config.PublicBaseURL != "" {
		return strings.TrimRight(s.Config.PublicBaseURL, "/")
	}

	scheme := "https"
	if r.TLS == nil && !requestWasHTTPS(r) {
		scheme = "http"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return scheme + "://" + host
}

func requestWasHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.Contains(r.Header.Get("CF-Visitor"), `"scheme":"https"`) {
		return true
	}
	if proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); strings.EqualFold(proto, "https") {
		return true
	}
	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		return strings.Contains(strings.ToLower(fwd), "proto=https")
	}
	return false
}

// getSystemUserID returns a system user ID for dynamic client registration.
func getSystemUserID(s *Server) uuid.UUID {
	return uuid.MustParse("a0000000-0000-0000-0000-000000000001")
}
