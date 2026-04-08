package mcp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubHandler struct{}

func (stubHandler) HandleJSONRPC(req JSONRPCRequest) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"method": req.Method,
		},
	}
}

func TestRunStdioHandler(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n")
	var output bytes.Buffer
	if err := RunStdioHandler(stubHandler{}, input, &output); err != nil {
		t.Fatalf("RunStdioHandler: %v", err)
	}
	if !strings.Contains(output.String(), "\"method\":\"ping\"") {
		t.Fatalf("unexpected stdio output: %s", output.String())
	}
}

func TestServeHTTPHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"initialize"}`))
	rec := httptest.NewRecorder()
	ServeHTTPHandler(stubHandler{}, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "\"method\":\"initialize\"") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
