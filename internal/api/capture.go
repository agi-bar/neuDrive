package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/logger"
)

const captureBodyLimit = 128 * 1024

var captureSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)

type captureRecord struct {
	TimestampUTC string                 `json:"timestamp_utc"`
	RequestID    string                 `json:"request_id,omitempty"`
	Kind         string                 `json:"kind"`
	Source       string                 `json:"source"`
	Request      captureRequestDetails  `json:"request"`
	Response     captureResponseDetails `json:"response"`
}

type captureRequestDetails struct {
	Method        string              `json:"method"`
	Scheme        string              `json:"scheme"`
	Host          string              `json:"host"`
	Path          string              `json:"path"`
	RawQuery      string              `json:"raw_query,omitempty"`
	Query         map[string][]string `json:"query,omitempty"`
	RemoteAddr    string              `json:"remote_addr,omitempty"`
	UserAgent     string              `json:"user_agent,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          string              `json:"body,omitempty"`
	BodyTruncated bool                `json:"body_truncated,omitempty"`
	ParsedBody    any                 `json:"parsed_body,omitempty"`
}

type captureResponseDetails struct {
	Status int                 `json:"status"`
	Header map[string][]string `json:"header,omitempty"`
}

func CaptureOAuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	enabled := cfg != nil && cfg.CaptureOAuth
	dir := "tmp/oauth-captures"
	if cfg != nil && strings.TrimSpace(cfg.CaptureDir) != "" {
		dir = cfg.CaptureDir
	}

	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			kind, ok := captureKind(r.URL.Path)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			body, truncated, err := readCaptureBody(r)
			if err != nil {
				logger.FromContext(r.Context()).Warn("capture read failed", "error", err)
			}

			ww := &captureResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(ww, r)

			record := captureRecord{
				TimestampUTC: time.Now().UTC().Format(time.RFC3339Nano),
				RequestID:    logger.RequestIDFromContext(r.Context()),
				Kind:         kind,
				Source:       inferCaptureSource(r, body),
				Request: captureRequestDetails{
					Method:        r.Method,
					Scheme:        captureScheme(r),
					Host:          r.Host,
					Path:          r.URL.Path,
					RawQuery:      r.URL.RawQuery,
					Query:         cloneValues(r.URL.Query()),
					RemoteAddr:    r.RemoteAddr,
					UserAgent:     r.UserAgent(),
					Headers:       cloneHeaders(r.Header),
					Body:          string(body),
					BodyTruncated: truncated,
					ParsedBody:    parseCapturedBody(body, r.Header.Get("Content-Type")),
				},
				Response: captureResponseDetails{
					Status: ww.statusCode,
					Header: cloneHeaders(ww.Header()),
				},
			}

			if err := writeCaptureRecord(dir, record); err != nil {
				logger.FromContext(r.Context()).Warn("capture write failed", "error", err)
			}
		})
	}
}

func captureKind(path string) (string, bool) {
	switch {
	case path == "/mcp":
		return "mcp", true
	case path == "/oauth/register":
		return "oauth_register", true
	case path == "/oauth/authorize":
		return "oauth_authorize", true
	case path == "/oauth/token":
		return "oauth_token", true
	case path == "/.well-known/oauth-protected-resource" || strings.HasPrefix(path, "/.well-known/oauth-protected-resource/"):
		return "oauth_protected_resource", true
	case path == "/.well-known/oauth-authorization-server" || strings.HasPrefix(path, "/.well-known/oauth-authorization-server/"):
		return "oauth_authorization_server", true
	case path == "/.well-known/openid-configuration" || strings.HasPrefix(path, "/.well-known/openid-configuration/"):
		return "openid_configuration", true
	default:
		return "", false
	}
}

func readCaptureBody(r *http.Request) ([]byte, bool, error) {
	if r.Body == nil {
		return nil, false, nil
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, false, err
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))

	if len(raw) <= captureBodyLimit {
		return raw, false, nil
	}
	return raw[:captureBodyLimit], true, nil
}

func parseCapturedBody(body []byte, contentType string) any {
	if len(body) == 0 {
		return nil
	}

	switch {
	case strings.Contains(contentType, "application/json"):
		var out any
		if err := json.Unmarshal(body, &out); err == nil {
			return out
		}
	case strings.Contains(contentType, "application/x-www-form-urlencoded"):
		if values, err := url.ParseQuery(string(body)); err == nil {
			return cloneValues(values)
		}
	}

	return nil
}

func cloneHeaders(header http.Header) map[string][]string {
	if len(header) == 0 {
		return nil
	}

	out := make(map[string][]string, len(header))
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := header.Values(key)
		out[key] = append([]string{}, values...)
	}
	return out
}

func cloneValues(values url.Values) map[string][]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string][]string, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = append([]string{}, values[key]...)
	}
	return out
}

func captureScheme(r *http.Request) string {
	if requestWasHTTPS(r) {
		return "https"
	}
	return "http"
}

func inferCaptureSource(r *http.Request, body []byte) string {
	userAgent := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(userAgent, "claude-code"):
		return "claude-code"
	case strings.Contains(userAgent, "claude-user"):
		return "claude-web"
	case strings.Contains(userAgent, "cursor/"):
		return "cursor"
	case strings.Contains(userAgent, "gemini"):
		return "gemini-cli"
	case strings.Contains(userAgent, "chatgpt"):
		return "chatgpt"
	case strings.Contains(userAgent, "openai"):
		return "openai"
	case strings.Contains(userAgent, "codex"):
		return "codex"
	}

	candidates := []string{
		r.URL.Query().Get("client_id"),
		r.URL.Query().Get("redirect_uri"),
	}

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if values, err := url.ParseQuery(string(body)); err == nil {
			candidates = append(candidates, values.Get("client_id"), values.Get("redirect_uri"))
		}
	} else if strings.Contains(contentType, "application/json") {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			for _, key := range []string{"client_id", "redirect_uri"} {
				if value, ok := payload[key].(string); ok {
					candidates = append(candidates, value)
				}
			}
			if value, ok := payload["client_name"].(string); ok {
				candidates = append(candidates, value)
			}
			if params, ok := payload["params"].(map[string]any); ok {
				if clientInfo, ok := params["clientInfo"].(map[string]any); ok {
					if value, ok := clientInfo["name"].(string); ok {
						candidates = append(candidates, value)
					}
				}
			}
		}
	}

	joined := strings.ToLower(strings.Join(candidates, " "))
	switch {
	case strings.Contains(joined, "codex"):
		return "codex"
	case strings.Contains(joined, "cursor-vscode") || strings.Contains(joined, "cursor://anysphere.cursor-mcp") || strings.Contains(joined, "\"cursor\""):
		return "cursor"
	case strings.Contains(joined, "gemini"):
		return "gemini-cli"
	case strings.Contains(joined, "claude-code"):
		return "claude-code"
	case strings.Contains(joined, "mcp-oauth-client-metadata") || strings.Contains(joined, "claude.ai/api/mcp/auth_callback") || strings.Contains(joined, "anthropic/toolbox"):
		return "claude-web"
	case strings.Contains(joined, "chatgpt") || strings.Contains(joined, "chat.openai.com") || strings.Contains(joined, "chatgpt.com"):
		return "chatgpt"
	case strings.Contains(joined, "claude.ai") || strings.Contains(joined, "claude.com"):
		return "claude"
	case strings.Contains(joined, "openai.com") || strings.Contains(joined, "openai-mcp"):
		return "chatgpt"
	case strings.Contains(joined, "cursor.com"):
		return "cursor"
	case strings.Contains(joined, "copilot"):
		return "copilot"
	case strings.Contains(joined, "windsurf"):
		return "windsurf"
	default:
		return "unknown"
	}
}

func writeCaptureRecord(dir string, record captureRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	fileName := fmt.Sprintf(
		"%s_%s_%s_%s.json",
		time.Now().UTC().Format("20060102T150405.000000000Z"),
		sanitizeCaptureName(record.Kind),
		sanitizeCaptureName(record.Source),
		sanitizeCaptureName(record.RequestID),
	)
	path := filepath.Join(dir, fileName)

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o600)
}

func sanitizeCaptureName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "none"
	}
	return strings.Trim(captureSanitizer.ReplaceAllString(value, "-"), "-")
}

type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *captureResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
