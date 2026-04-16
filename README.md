English | [简体中文](README.zh-CN.md)

# neuDrive

**Identity and trust infrastructure for the AI era**

> Let every AI agent know who you are, remember your context, and act on your behalf.

neuDrive is an independent hub layer for people and AI agents. Claude, ChatGPT, Codex, Cursor, Gemini, Feishu, and other agents can share identity, memory, skills, secrets, and communication through one hub.

Your identity, preferences, secrets, and skills follow the person, not the platform.

The system exposes one canonical virtual tree together with typed APIs, file-tree access, and `snapshot/changes` sync interfaces.

- Writing preferences captured in Claude can immediately help GPT later the same day.
- API keys stored in the vault can be used safely by authorized agents.
- Agents can message each other, collaborate, and hand work off without making you the relay.
- One Hub ID can travel across AI platforms.

Hosted service examples in this repo use:

- Hub URL: `https://neudrive.ai`
- MCP URL: `https://neudrive.ai/mcp`

## Connection Modes

Start with the first mode that matches your situation:

| Order | Mode | Best for | Guide |
|-------|------|----------|-------|
| 1 | **Web / Desktop Apps** | The fastest path for Claude, ChatGPT, Cursor, and Windsurf through hosted neuDrive with browser auth | [Open guide](docs/setup.md#web-and-desktop-apps) |
| 2 | **CLI Apps** | Claude Code, Codex CLI, Gemini CLI, and Cursor Agent with remote HTTP MCP + OAuth | [Open guide](docs/setup.md#cli-apps) |
| 3 | **Local Mode** | Repo-first local development, LAN setups, or any environment without a public HTTPS URL yet | [Open guide](docs/setup.md#local-mode) |
| 4 | **Advanced Mode / GPT Actions / Adapters** | Generic HTTP MCP clients, custom GPTs, and webhook-style integrations such as Feishu | [Open guide](docs/setup.md#advanced-mode) |

More docs:

- [Token Management](docs/setup.md#token-management)
- [Bundle Sync](docs/sync.md)
- [SDK / HTTP API](docs/reference.md#sdk)

## Local CLI Quick Start

Examples below use `neu`; the `neudrive` alias still works.

```bash
git clone https://github.com/agi-bar/neudrive.git
cd neudrive
./tools/install-neudrive.sh

neu status
neu platform ls
neu connect claude
neu browse
```

Detailed CLI usage: [CLI Guide](docs/cli.md)

## Login To Hosted Cloud

Use the hosted `official` profile when you want the cloud hub at `https://neudrive.ai` behind Claude and other web apps, or when you want to move data through the hosted dashboard and sync flows.

```bash
neu remote login official
```

This opens a browser login flow and saves the `official` profile locally. After that, follow [Web / Desktop Apps](docs/setup.md#web-and-desktop-apps) to connect Claude, ChatGPT, Cursor, or Windsurf to the hosted MCP endpoint.

## Documentation

- [Setup Guide](docs/setup.md)
- [CLI Guide](docs/cli.md)
- [Reference](docs/reference.md)
- [Chinese Setup Guide](docs/setup.zh-CN.md)
- [Chinese CLI Guide](docs/cli.zh-CN.md)
- [Chinese Reference](docs/reference.zh-CN.md)
- [Chinese README](README.zh-CN.md)
- [Product design document](docs/design.md) (currently Chinese)
- [Bundle Sync guide](docs/sync.md) (currently Chinese)
- [Prod-like acceptance runbook](docs/sync-prodlike-acceptance.md) (currently Chinese)
- [Security and resource audit](docs/sync-audit.md) (currently Chinese)
- [CLI test matrix](docs/cli-test-matrix.md) (currently Chinese)
