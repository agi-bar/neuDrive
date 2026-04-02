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
)

// ---------------------------------------------------------------------------
// Remote MCP endpoint — Streamable HTTP transport for Claude.ai Connectors
//
// Spec: https://modelcontextprotocol.io/docs/concepts/transports#streamable-http
//
// POST /mcp — Client sends JSON-RPC request, server returns JSON response
// GET  /mcp — Server-initiated SSE stream (optional, returns 405 for now)
// ---------------------------------------------------------------------------

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	// CORS headers for Claude.ai
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
		// SSE stream for server-initiated messages (optional)
		w.WriteHeader(http.StatusMethodNotAllowed)
	case http.MethodDelete:
		// Session termination
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMCPPost(w http.ResponseWriter, r *http.Request) {
	// 1. Extract and validate Bearer token
	tokenStr, err := auth.ExtractTokenFromHeader(r)
	if err != nil {
		// Also try X-API-Key header
		tokenStr = r.Header.Get("X-API-Key")
	}

	if tokenStr == "" || !strings.HasPrefix(tokenStr, "aht_") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32000, "message": "authentication required: provide Authorization: Bearer aht_xxx"},
		})
		return
	}

	scopedToken, err := s.TokenService.ValidateToken(r.Context(), tokenStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32000, "message": "invalid or expired token"},
		})
		return
	}

	// 2. Check rate limit
	if err := s.TokenService.CheckRateLimit(r.Context(), scopedToken); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32000, "message": err.Error()},
		})
		return
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

	// 4. Create MCPServer instance for this request
	mcpServer := s.createMCPServer(scopedToken)

	// 5. Handle notification (no response needed)
	if req.Method == "notifications/initialized" || strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// 6. Process JSON-RPC request
	resp := mcpServer.HandleJSONRPC(req)

	// 7. Add session ID on initialize
	if req.Method == "initialize" {
		sessionID := generateMCPSessionID()
		w.Header().Set("Mcp-Session-Id", sessionID)
	}

	// 8. Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// createMCPServer builds an MCPServer with all service dependencies injected.
func (s *Server) createMCPServer(token *models.ScopedToken) *mcp.MCPServer {
	return &mcp.MCPServer{
		UserID:      token.UserID,
		TrustLevel:  token.MaxTrustLevel,
		Scopes:      token.Scopes,
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
}

func generateMCPSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("session-%d", 0)
	}
	return hex.EncodeToString(b)
}
