package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/logger"
)

func TestInferCaptureSourceFromCapturedFixtures(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		expected string
	}{
		{
			name:     "codex dynamic register",
			fixture:  "testdata/oauth/codex/oauth_register.json",
			expected: "codex",
		},
		{
			name:     "claude code initialize",
			fixture:  "testdata/oauth/claude-code/mcp_initialize.json",
			expected: "claude-code",
		},
		{
			name:     "claude code token from metadata client id",
			fixture:  "testdata/oauth/claude-code/oauth_token_authorization_code.json",
			expected: "claude-code",
		},
		{
			name:     "claude web token from metadata client id",
			fixture:  "testdata/oauth/claude-web/oauth_token_authorization_code.json",
			expected: "claude-web",
		},
		{
			name:     "claude web authenticated initialize",
			fixture:  "testdata/oauth/claude-web/mcp_initialize_authenticated.json",
			expected: "claude-web",
		},
		{
			name:     "chatgpt dynamic register",
			fixture:  "testdata/oauth/chatgpt/oauth_register.json",
			expected: "chatgpt",
		},
		{
			name:     "chatgpt authenticated initialize",
			fixture:  "testdata/oauth/chatgpt/mcp_initialize_authenticated.json",
			expected: "chatgpt",
		},
		{
			name:     "gemini cli dynamic register",
			fixture:  "testdata/oauth/gemini-cli/oauth_register.json",
			expected: "gemini-cli",
		},
		{
			name:     "gemini cli authenticated initialize",
			fixture:  "testdata/oauth/gemini-cli/mcp_initialize_authenticated.json",
			expected: "gemini-cli",
		},
		{
			name:     "cursor desktop dynamic register",
			fixture:  "testdata/oauth/cursor-desktop/oauth_register.json",
			expected: "cursor",
		},
		{
			name:     "cursor desktop authenticated initialize",
			fixture:  "testdata/oauth/cursor-desktop/mcp_initialize_authenticated.json",
			expected: "cursor",
		},
		{
			name:     "cursor agent dynamic register",
			fixture:  "testdata/oauth/cursor-agent/oauth_register.json",
			expected: "cursor",
		},
		{
			name:     "cursor agent authenticated initialize",
			fixture:  "testdata/oauth/cursor-agent/mcp_initialize_authenticated.json",
			expected: "cursor",
		},
		{
			name:     "windsurf dynamic register",
			fixture:  "testdata/oauth/windsurf/oauth_register.json",
			expected: "windsurf",
		},
		{
			name:     "windsurf authenticated initialize",
			fixture:  "testdata/oauth/windsurf/mcp_initialize_authenticated.json",
			expected: "windsurf",
		},
		{
			name:     "codex initialize",
			fixture:  "testdata/oauth/codex/mcp_initialize.json",
			expected: "codex",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newRequestFromFixture(t, tc.fixture)
			body, _, err := readCaptureBody(req)
			if err != nil {
				t.Fatalf("readCaptureBody: %v", err)
			}

			if got := inferCaptureSource(req, body); got != tc.expected {
				t.Fatalf("expected source %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestInferCaptureSourceFromSyntheticSignals(t *testing.T) {
	cases := []struct {
		name        string
		userAgent   string
		contentType string
		body        string
		headers     map[string]string
		expected    string
	}{
		{
			name:        "explicit platform header wins",
			contentType: "application/json",
			body:        `{"params":{"clientInfo":{"name":"codex-mcp-client"}}}`,
			headers:     map[string]string{"X-NeuDrive-Platform": "perplexity"},
			expected:    "perplexity",
		},
		{
			name:        "copilot client name",
			contentType: "application/json",
			body:        `{"client_name":"GitHub Copilot MCP Client"}`,
			expected:    "copilot",
		},
		{
			name:        "kimi redirect uri",
			contentType: "application/json",
			body:        `{"redirect_uri":"https://kimi.moonshot.cn/oauth/callback"}`,
			expected:    "kimi",
		},
		{
			name:        "deepseek origin",
			contentType: "application/json",
			body:        `{"client_name":"Remote MCP App"}`,
			headers:     map[string]string{"Origin": "https://chat.deepseek.com"},
			expected:    "deepseek",
		},
		{
			name:        "qwen host",
			contentType: "application/json",
			body:        `{"client_id":"https://tongyi.aliyun.com/mcp/client"}`,
			expected:    "qwen",
		},
		{
			name:        "zhipu host",
			contentType: "application/json",
			body:        `{"client_id":"https://open.bigmodel.cn/oauth/client"}`,
			expected:    "zhipu",
		},
		{
			name:        "feishu app name",
			contentType: "application/json",
			body:        `{"client_name":"Feishu Bot Adapter"}`,
			expected:    "feishu",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://example.test/mcp", strings.NewReader(tc.body))
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}
			body, _, err := readCaptureBody(req)
			if err != nil {
				t.Fatalf("readCaptureBody: %v", err)
			}
			if got := inferCaptureSource(req, body); got != tc.expected {
				t.Fatalf("expected source %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestCaptureOAuthMiddlewareWritesDetailedRequestFile(t *testing.T) {
	logger.Init("debug", "text")

	dir := t.TempDir()
	cfg := &config.Config{
		CaptureOAuth: true,
		CaptureDir:   dir,
	}

	handler := CaptureOAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusCreated)
	}))

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"fixture-code"},
		"code_verifier": {"fixture-verifier"},
		"redirect_uri":  {"http://127.0.0.1:51431/callback"},
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.test/oauth/token?flow=pkce", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic dGVzdC1jbGllbnQ6dGVzdC1zZWNyZXQ=")
	req = req.WithContext(logger.WithRequestID(req.Context(), "req-fixture-1"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 capture file, got %d", len(entries))
	}

	path := filepath.Join(dir, entries[0].Name())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var record captureRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if record.Kind != "oauth_token" {
		t.Fatalf("expected kind oauth_token, got %q", record.Kind)
	}
	if record.Request.Method != http.MethodPost {
		t.Fatalf("expected method POST, got %q", record.Request.Method)
	}
	if record.Request.Query["flow"][0] != "pkce" {
		t.Fatalf("expected captured query param, got %v", record.Request.Query)
	}
	if record.Request.Headers["Authorization"][0] != "Basic dGVzdC1jbGllbnQ6dGVzdC1zZWNyZXQ=" {
		t.Fatalf("expected Authorization header captured, got %v", record.Request.Headers["Authorization"])
	}
	parsedBody, ok := record.Request.ParsedBody.(map[string]any)
	if !ok {
		t.Fatalf("expected parsed form body, got %T", record.Request.ParsedBody)
	}
	if values, ok := parsedBody["grant_type"].([]any); !ok || len(values) != 1 || values[0] != "authorization_code" {
		t.Fatalf("expected parsed grant_type, got %v", parsedBody["grant_type"])
	}
	if record.Response.Status != http.StatusCreated {
		t.Fatalf("expected response status 201, got %d", record.Response.Status)
	}
}
