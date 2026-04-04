package mcp

import "testing"

func TestGenerateHTTPOAuthConfig(t *testing.T) {
	cfg := GenerateHTTPOAuthConfig("https://hub.example.com")

	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcpServers map, got %T", cfg["mcpServers"])
	}

	server, ok := mcpServers["agenthub"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agenthub server map, got %T", mcpServers["agenthub"])
	}

	if server["type"] != "http" {
		t.Fatalf("expected type=http, got %v", server["type"])
	}
	if server["url"] != "https://hub.example.com/mcp" {
		t.Fatalf("unexpected url: %v", server["url"])
	}
	if _, exists := server["headers"]; exists {
		t.Fatal("oauth config should not include static headers")
	}
}

func TestGenerateHTTPBearerConfig(t *testing.T) {
	cfg := GenerateHTTPBearerConfig("https://hub.example.com", "aht_test")

	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcpServers map, got %T", cfg["mcpServers"])
	}

	server, ok := mcpServers["agenthub"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agenthub server map, got %T", mcpServers["agenthub"])
	}

	headers, ok := server["headers"].(map[string]string)
	if !ok {
		t.Fatalf("expected headers map, got %T", server["headers"])
	}

	if headers["Authorization"] != "Bearer aht_test" {
		t.Fatalf("unexpected authorization header: %v", headers["Authorization"])
	}
}

func TestGenerateHTTPConfigAlias(t *testing.T) {
	cfg := GenerateHTTPConfig("https://hub.example.com", "aht_alias")

	mcpServers := cfg["mcpServers"].(map[string]interface{})
	server := mcpServers["agenthub"].(map[string]interface{})
	headers := server["headers"].(map[string]string)

	if headers["Authorization"] != "Bearer aht_alias" {
		t.Fatalf("unexpected authorization header: %v", headers["Authorization"])
	}
}
