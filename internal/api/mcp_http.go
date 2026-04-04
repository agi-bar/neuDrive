package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/mcp"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Remote MCP endpoint — Streamable HTTP transport for MCP clients such as
// Claude and ChatGPT apps/connectors.
// ---------------------------------------------------------------------------

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Session-Id, MCP-Protocol-Version")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleMCPPost(w, r)
	case http.MethodGet:
		w.WriteHeader(http.StatusMethodNotAllowed)
	case http.MethodDelete:
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMCPPost(w http.ResponseWriter, r *http.Request) {
	baseURL := s.baseURL(r)
	resourceMetadataURL := baseURL + "/.well-known/oauth-protected-resource"

	// 1. Extract token
	tokenStr, err := auth.ExtractTokenFromHeader(r)
	if err != nil {
		tokenStr = r.Header.Get("X-API-Key")
	}

	if tokenStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+resourceMetadataURL+`"`)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32000, "message": "authentication required"},
		})
		return
	}

	// 2. Authenticate — try scoped token first, then OAuth JWT
	var userID uuid.UUID
	var trustLevel int
	var scopes []string

	if strings.HasPrefix(tokenStr, "aht_") {
		// Scoped token (from Hub dashboard)
		st, err := s.TokenService.ValidateToken(r.Context(), tokenStr)
		if err != nil {
			mcpUnauthorized(w, resourceMetadataURL, "invalid or expired scoped token")
			return
		}
		if err := s.TokenService.CheckRateLimit(r.Context(), st); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": -32000, "message": err.Error()}})
			return
		}
		userID = st.UserID
		trustLevel = st.MaxTrustLevel
		scopes = st.Scopes
	} else {
		// OAuth JWT (from Claude.ai Custom Connector)
		claims, err := auth.ValidateToken(tokenStr, s.JWTSecret)
		if err != nil {
			mcpUnauthorized(w, resourceMetadataURL, "invalid or expired OAuth token")
			return
		}
		userID = claims.UserID
		trustLevel = models.TrustLevelFull
		scopes = []string{models.ScopeAdmin}
	}

	// 3. Decode JSON-RPC request
	var req mcp.JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcp.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &mcp.RPCError{Code: -32700, Message: "parse error"},
		})
		return
	}

	// 4. Handle notifications (no response)
	if strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// 5. Create MCPServer and handle request
	mcpServer := &mcp.MCPServer{
		UserID:      userID,
		TrustLevel:  trustLevel,
		Scopes:      scopes,
		FileTree:    s.FileTreeService,
		Vault:       s.VaultService,
		VaultCrypto: s.Vault,
		Memory:      s.MemoryService,
		Project:     s.ProjectService,
		Inbox:       s.InboxService,
		Device:      s.DeviceService,
		Dashboard:   s.DashboardService,
		Import:      s.ImportService,
	}

	resp := mcpServer.HandleJSONRPC(req)

	// 6. Session ID on initialize
	if req.Method == "initialize" {
		w.Header().Set("Mcp-Session-Id", generateMCPSessionID())
	}

	// 7. Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func mcpUnauthorized(w http.ResponseWriter, resourceMetadataURL, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+resourceMetadataURL+`", error="invalid_token"`)
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"error":   map[string]any{"code": -32000, "message": msg},
	})
}

func generateMCPSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("session-%d", 0)
	}
	return hex.EncodeToString(b)
}
