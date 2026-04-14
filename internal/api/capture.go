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
	"github.com/agi-bar/neudrive/internal/services"
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

type sourcePattern struct {
	source       string
	textContains []string
	hostContains []string
}

var requestSourcePatterns = []sourcePattern{
	{source: "claude-code", textContains: []string{"claude-code", "claude code"}},
	{source: "claude-web", textContains: []string{"claude-user", "anthropic/toolbox", "mcp-oauth-client-metadata", "claude.ai/api/mcp/auth_callback"}, hostContains: []string{"claude.ai", "claude.com"}},
	{source: "codex", textContains: []string{"codex-mcp-client", "codex cli", "codex"}},
	{source: "chatgpt", textContains: []string{"openai-mcp (chatgpt)", "chatgpt", "chat.openai.com"}, hostContains: []string{"chatgpt.com", "chat.openai.com"}},
	{source: "gemini-cli", textContains: []string{"gemini-cli-mcp-client", "gemini cli mcp client", "gemini cli"}},
	{source: "cursor", textContains: []string{"cursor-vscode", "anysphere.cursor-mcp", "cursor agent", "cursor"}, hostContains: []string{"cursor.com"}},
	{source: "windsurf", textContains: []string{"windsurf"}, hostContains: []string{"windsurf.com", "codeium.com"}},
	{source: "copilot", textContains: []string{"github copilot", "copilot"}},
	{source: "perplexity", textContains: []string{"perplexity"}, hostContains: []string{"perplexity.ai"}},
	{source: "kimi", textContains: []string{"kimi"}, hostContains: []string{"kimi.com", "moonshot.cn"}},
	{source: "deepseek", textContains: []string{"deepseek"}, hostContains: []string{"deepseek.com"}},
	{source: "qwen", textContains: []string{"qwen", "tongyi"}, hostContains: []string{"tongyi.aliyun.com", "dashscope.aliyuncs.com"}},
	{source: "zhipu", textContains: []string{"zhipu", "bigmodel", "chatglm"}, hostContains: []string{"bigmodel.cn", "z.ai"}},
	{source: "minimax", textContains: []string{"minimax"}, hostContains: []string{"minimax.chat", "minimax.io"}},
	{source: "feishu", textContains: []string{"feishu", "lark"}, hostContains: []string{"feishu.cn", "larksuite.com"}},
	{source: "open-webui", textContains: []string{"open webui", "openwebui"}},
	{source: "openai", textContains: []string{"openai"}, hostContains: []string{"openai.com"}},
	{source: "gemini", textContains: []string{"gemini"}, hostContains: []string{"gemini.google.com"}},
	{source: "claude", textContains: []string{"claude"}, hostContains: []string{"claude.ai", "claude.com"}},
}

func inferCaptureSource(r *http.Request, body []byte) string {
	if r == nil {
		return "unknown"
	}
	if explicit := services.NormalizeSource(r.Header.Get("X-NeuDrive-Platform")); explicit != "" {
		return explicit
	}
	if explicit := services.NormalizeSource(r.Header.Get("X-NeuDrive-Source")); explicit != "" {
		return explicit
	}

	textSignals := collectRequestTextSignals(r, body)
	hostSignals := collectRequestHostSignals(r, textSignals)
	joined := strings.ToLower(strings.Join(textSignals, " "))
	for _, pattern := range requestSourcePatterns {
		if containsAny(joined, pattern.textContains) || hostContainsAny(hostSignals, pattern.hostContains) {
			return pattern.source
		}
	}
	return "unknown"
}

func collectRequestTextSignals(r *http.Request, body []byte) []string {
	signals := []string{strings.ToLower(r.UserAgent())}
	for _, key := range []string{"Origin", "Referer"} {
		if value := strings.TrimSpace(r.Header.Get(key)); value != "" {
			signals = append(signals, strings.ToLower(value))
		}
	}
	for key, values := range r.URL.Query() {
		signals = append(signals, strings.ToLower(key))
		for _, value := range values {
			signals = append(signals, strings.ToLower(value))
		}
	}
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if values, err := url.ParseQuery(string(body)); err == nil {
			for key, items := range values {
				signals = append(signals, strings.ToLower(key))
				for _, value := range items {
					signals = append(signals, strings.ToLower(value))
				}
			}
		}
	} else if strings.Contains(contentType, "application/json") {
		var payload any
		if err := json.Unmarshal(body, &payload); err == nil {
			signals = appendJSONSignals(signals, payload)
		}
	}
	return signals
}

func appendJSONSignals(dst []string, value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			dst = append(dst, strings.ToLower(key))
			dst = appendJSONSignals(dst, typed[key])
		}
	case []any:
		for _, item := range typed {
			dst = appendJSONSignals(dst, item)
		}
	case string:
		dst = append(dst, strings.ToLower(typed))
	}
	return dst
}

func collectRequestHostSignals(r *http.Request, values []string) []string {
	hosts := []string{}
	appendHost := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
			hosts = append(hosts, strings.ToLower(parsed.Hostname()))
			return
		}
		if strings.Contains(raw, ".") && !strings.Contains(raw, " ") {
			hosts = append(hosts, strings.Trim(strings.ToLower(raw), `/`))
		}
	}
	for _, key := range []string{"Origin", "Referer"} {
		appendHost(r.Header.Get(key))
	}
	for _, value := range values {
		appendHost(value)
	}
	sort.Strings(hosts)
	return hosts
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func hostContainsAny(hosts []string, needles []string) bool {
	for _, host := range hosts {
		for _, needle := range needles {
			if strings.Contains(host, needle) {
				return true
			}
		}
	}
	return false
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
