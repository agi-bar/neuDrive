package api

import (
	"net/http"
)

// GET /api/gpt/setup — returns GPT Actions setup instructions and schema URL.
func (s *Server) handleGPTSetup(w http.ResponseWriter, r *http.Request) {
	baseURL := "https://hub.agibar.ai"

	// Allow override via X-Forwarded-Host or Host header for dev environments.
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			scheme = "https"
		}
		baseURL = scheme + "://" + fwd
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schema_url": baseURL + "/docs/gpt-actions-schema.yaml",
		"auth_type":  "bearer",
		"token_prefix": "aht_",
		"instructions": map[string]string{
			"step_1": "在 Agent Hub 设置页面创建一个 Token（推荐 Agent 完整权限）",
			"step_2": "在 ChatGPT 中打开 '创建 GPT' → '配置' → 'Actions'",
			"step_3": "点击 'Import from URL'，粘贴 schema_url",
			"step_4": "在 Authentication 选择 'API Key'，Auth Type 选 'Bearer'，粘贴你的 Token",
			"step_5": "保存并测试",
		},
		"endpoints_base": baseURL + "/gpt",
	})
}
