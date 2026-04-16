English | [简体中文](setup.zh-CN.md)

# neuDrive Setup Guide

This guide mirrors the connection categories shown in the dashboard's setup page. Use it after your neuDrive deployment is already running and you want concrete commands or config templates for a specific platform.

The examples below use:

- Hub URL: `https://neudrive.ai`
- MCP URL: `https://neudrive.ai/mcp`
- Scoped token environment variable: `NEUDRIVE_TOKEN`

If you are currently running on a local development address such as `http://localhost:8080`, only **Local Mode** is usually appropriate right away. Web / Desktop Apps and CLI Apps generally need a publicly reachable HTTPS URL.

## Web and Desktop Apps

These paths are best when the connection is created from a graphical interface, including browser-based Apps / Connectors and desktop applications such as Cursor and Windsurf.

### Claude Connectors

1. Sign in to the Claude web app and open `Settings -> Connectors -> Go to Customize`.
2. Click `Add custom connector`.
3. Set `Remote MCP Server URL` to `https://neudrive.ai/mcp`.
4. Save and click `Connect`.
5. Your browser will open the neuDrive sign-in and authorization flow; after approval, return to Claude.

### ChatGPT Apps

1. Sign in to ChatGPT and open `Settings -> Apps`.
2. In `Advanced settings`, click `Create app`.
3. Set `MCP Server URL` to `https://neudrive.ai/mcp`.
4. Follow the prompts to finish neuDrive sign-in and authorization.

If you do not see the `Apps` entry yet, your plan or rollout cohort probably does not have access to it yet.

### Cursor Desktop

You can add Remote MCP directly in the UI or write the config file manually.

```json
{
  "mcpServers": {
    "neudrive": {
      "url": "https://neudrive.ai/mcp"
    }
  }
}
```

Recommended steps:

1. Open `Settings -> Tools & MCPs -> Add Custom MCP`.
2. Set `Remote MCP Server URL` to `https://neudrive.ai/mcp`.
3. Click `Connect` or `Authenticate`.
4. Your browser will open the neuDrive sign-in and authorization page; when it is complete, return to Cursor.

### Windsurf Desktop

Windsurf currently connects to remote MCP primarily through its config file:

```json
{
  "mcpServers": {
    "neudrive": {
      "serverUrl": "https://neudrive.ai/mcp"
    }
  }
}
```

Recommended steps:

1. Open `Windsurf Settings -> Cascade`.
2. In the `MCP Servers` section, click `Open MCP Marketplace`.
3. Click the config icon and open `~/.codeium/windsurf/mcp_config.json`.
4. Add the `neudrive` config shown above and save it.
5. Click `Open`, then complete neuDrive sign-in and authorization.

## CLI Apps

These paths are best for users who work from the terminal. They connect to neuDrive through remote HTTP MCP plus OAuth.

### Claude Code

```bash
claude mcp add -s user --transport http neudrive https://neudrive.ai/mcp
```

Then run this inside Claude Code:

```text
/mcp
```

Then follow the browser authorization flow.

### Codex CLI

```bash
codex mcp add neudrive --url https://neudrive.ai/mcp
codex mcp login neudrive
codex mcp list
```

### Gemini CLI

```bash
gemini mcp add --transport http neudrive https://neudrive.ai/mcp
```

Then run this inside Gemini:

```text
/mcp auth neudrive
```

Important: `gemini mcp add` must include `--transport http`, or Gemini may treat the URL as a local command instead of a remote MCP server.

### Cursor Agent

First write this into `.cursor/mcp.json` or `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "neudrive": {
      "url": "https://neudrive.ai/mcp"
    }
  }
}
```

Then run:

```bash
cursor-agent mcp login neudrive
cursor-agent mcp list
```

## Local Mode

Local mode is best for local development, internal-network environments, or any setup that does not yet have a public HTTPS URL. It connects through the local `neudrive-mcp` binary and a scoped token.

First prepare a token:

```bash
export NEUDRIVE_TOKEN=ndt_xxxxx
```

### Claude Code

```bash
claude mcp add -s user neudrive -- neudrive-mcp --token-env NEUDRIVE_TOKEN
```

### Codex CLI

```bash
codex mcp add neudrive -- neudrive-mcp --token-env NEUDRIVE_TOKEN
```

If you only want to inspect the setup and do not want to mint a token yet, open `Connection Setup -> Local Mode` in the dashboard. You can create and copy a mode-specific token there when you are ready.

## Advanced Mode

Advanced mode targets generic clients that support HTTP MCP. Prefer environment variables whenever possible, and only fall back to a static Bearer header when the client cannot read tokens from env.

```bash
export NEUDRIVE_TOKEN=ndt_xxxxx
```

Codex CLI can reference the environment variable directly so the secret does not need to be written into config:

```bash
codex mcp add neudrive --url https://neudrive.ai/mcp --bearer-token-env-var NEUDRIVE_TOKEN
```

For other clients, use a static Bearer configuration only if env-based auth is not supported yet.

## ChatGPT GPT Actions

If you want to connect neuDrive to a custom GPT, use GPT Actions:

1. Open ChatGPT and create a GPT.
2. Go to `Configure -> Actions`.
3. Set `OpenAPI Schema URL` to `https://neudrive.ai/gpt/openapi.json`.
4. Choose `Bearer Token` for authentication.
5. Use a scoped token as the Bearer token.

The recommended path is to create a dedicated token first in `Connection Setup -> Token Management`.

## Adapters

Adapters are meant for workspace platforms such as Feishu, DingTalk, and Slack. The currently documented example in this repo is the Feishu Bot Adapter.

### Feishu Bot Adapter

Callback URL format:

```text
https://neudrive.ai/api/adapters/feishu/<your-slug>/events
```

Server-side environment variables:

```bash
FEISHU_APP_ID=replace-with-your-app-id
FEISHU_APP_SECRET=replace-with-your-app-secret
FEISHU_VERIFICATION_TOKEN=replace-with-your-verification-token
FEISHU_ENCRYPT_KEY=replace-with-your-encrypt-key
```

Recommended steps:

1. Create a custom app in the Feishu developer console and enable bot capability.
2. Subscribe to `Messages and Groups -> Receive Messages v2.0`.
3. Choose `Send events to developer server`.
4. Use the callback URL above as the request URL.
5. Configure `FEISHU_APP_ID`, `FEISHU_APP_SECRET`, and `FEISHU_VERIFICATION_TOKEN` on the server.
6. It is strongly recommended to also configure `FEISHU_ENCRYPT_KEY` so signature validation and event decryption are enabled.

## Token Management

Token management is not a separate connection mode, but almost every non-OAuth path depends on it.

From the dashboard you can:

- Create scoped tokens
- Choose presets such as read-only, agent full, or sync
- Select scopes manually
- Rename, revoke, or rotate tokens

## Bundle Sync

For large skills, long-form documents, PNG / PDF assets, and other binary resources, prefer Bundle Sync instead of having AI write files one by one through MCP tools.

- [Bundle Sync guide](./sync.md)
- [Prod-like acceptance runbook](./sync-prodlike-acceptance.md)
- [Security and resource audit](./sync-audit.md)
