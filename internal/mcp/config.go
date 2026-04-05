package mcp

const DefaultTokenEnvVar = "AGENTHUB_TOKEN"

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

// GenerateStdioEnvConfig returns a stdio MCP config that reads the token from
// an environment variable at runtime instead of embedding it in args.
func GenerateStdioEnvConfig(binaryPath, tokenEnvVar string) map[string]interface{} {
	if tokenEnvVar == "" {
		tokenEnvVar = DefaultTokenEnvVar
	}
	return map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"agenthub": map[string]interface{}{
				"command": binaryPath,
				"args":    []string{"--token-env", tokenEnvVar},
			},
		},
	}
}

// GenerateHTTPOAuthConfig returns remote HTTP MCP config that relies on OAuth
// discovery and browser-based authorization instead of a static bearer token.
func GenerateHTTPOAuthConfig(baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"agenthub": map[string]interface{}{
				"type": "http",
				"url":  baseURL + "/mcp",
			},
		},
	}
}

// GenerateHTTPBearerConfig returns remote HTTP MCP config using a static bearer
// token in the Authorization header.
func GenerateHTTPBearerConfig(baseURL, token string) map[string]interface{} {
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

// GenerateHTTPConfig is kept as a backwards-compatible alias for the bearer
// token variant of remote HTTP MCP config.
func GenerateHTTPConfig(baseURL, token string) map[string]interface{} {
	return GenerateHTTPBearerConfig(baseURL, token)
}

// GenerateCLICommand returns the `claude mcp add` command string
func GenerateCLICommand(binaryPath, token string) string {
	return "claude mcp add agenthub --transport stdio -- " + binaryPath + " --token " + token
}

// GenerateCLIEnvCommand returns the `claude mcp add` command string for
// env-var-based stdio auth.
func GenerateCLIEnvCommand(binaryPath, tokenEnvVar string) string {
	if tokenEnvVar == "" {
		tokenEnvVar = DefaultTokenEnvVar
	}
	return "claude mcp add agenthub -- " + binaryPath + " --token-env " + tokenEnvVar
}
