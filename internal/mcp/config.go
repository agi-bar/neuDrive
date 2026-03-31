package mcp

// GenerateStdioConfig returns the Claude Code MCP config for stdio transport
func GenerateStdioConfig(binaryPath, token string) map[string]interface{} {
	return map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"agenthub": map[string]interface{}{
				"command": binaryPath,
				"args":    []string{"--token", token},
			},
		},
	}
}

// GenerateHTTPConfig returns the Claude Code MCP config for HTTP transport
func GenerateHTTPConfig(baseURL, token string) map[string]interface{} {
	return map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"agenthub": map[string]interface{}{
				"type": "http",
				"url":  baseURL + "/mcp",
				"headers": map[string]string{
					"Authorization": "Bearer " + token,
				},
			},
		},
	}
}

// GenerateCLICommand returns the `claude mcp add` command string
func GenerateCLICommand(binaryPath, token string) string {
	return "claude mcp add agenthub --transport stdio -- " + binaryPath + " --token " + token
}
