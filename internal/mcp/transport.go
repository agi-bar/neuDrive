package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// JSONRPCHandler is the minimal protocol surface shared by the HTTP and stdio
// MCP transports. Both the remote and sqlite-backed servers implement it.
type JSONRPCHandler interface {
	HandleJSONRPC(JSONRPCRequest) JSONRPCResponse
}

// RunStdioHandler runs any JSON-RPC handler on stdin/stdout.
func RunStdioHandler(handler JSONRPCHandler, stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeJSONLine(stdout, JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &RPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		resp := handler.HandleJSONRPC(req)
		if req.ID == nil && strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		writeJSONLine(stdout, resp)
	}
	return scanner.Err()
}

// ServeHTTPHandler exposes any JSON-RPC handler over a simple POST-only HTTP
// transport.
func ServeHTTPHandler(handler JSONRPCHandler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &RPCError{Code: -32700, Message: "parse error"},
		})
		return
	}

	resp := handler.HandleJSONRPC(req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Helper: write JSON line to writer
func writeJSONLine(w io.Writer, v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Fprintf(w, "%s\n", data)
}
