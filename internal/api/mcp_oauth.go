package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

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
	baseURL := getBaseURL(r)

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
			"search", "admin",
		},
		"bearer_methods_supported": []string{"header"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(metadata)
}

// handleAuthorizationServerMetadata serves OAuth 2.0 Authorization Server Metadata (RFC 8414).
func (s *Server) handleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	baseURL := getBaseURL(r)

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
			"search", "admin",
		},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "none"},
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

	var req struct {
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		GrantTypes              []string `json:"grant_types"`
		Scope                   string   `json:"scope"`
		ClientID                string   `json:"client_id"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
		ApplicationType         string   `json:"application_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

// getBaseURL extracts the base URL from the request.
func getBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

// getSystemUserID returns a system user ID for dynamic client registration.
func getSystemUserID(s *Server) uuid.UUID {
	return uuid.MustParse("a0000000-0000-0000-0000-000000000001")
}
