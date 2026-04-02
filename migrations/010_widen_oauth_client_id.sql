-- Widen client_id to support MCP Client ID Metadata Document URLs
-- (e.g., https://claude.ai/oauth/mcp/client-metadata.json)
ALTER TABLE oauth_apps ALTER COLUMN client_id TYPE VARCHAR(512);
