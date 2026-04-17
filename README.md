English | [简体中文](README.zh-CN.md)

# neuDrive

<p align="center">
  <img src="docs/assets/neudrive-logo.png" alt="neuDrive logo" width="320" />
</p>

**A personal hub for AI identity, memory, and trust**

> One place where Claude, ChatGPT, Codex, Cursor, and other agents can share who you are, what you prefer, and what they are allowed to do.

neuDrive gives one person one hub. Claude, ChatGPT, Codex, Cursor, Gemini, Feishu, and other agents can share identity, memory, skills, secrets, and communication through that hub instead of rebuilding context on every platform.

Your identity, preferences, secrets, and skills follow the person, not the platform.

Why people use it:

- Writing preferences captured in Claude can help GPT later the same day.
- Secrets live in one vault and stay available only to authorized agents.
- Agents can message each other, collaborate, and hand work off without making you the relay.
- One Hub ID can travel across AI platforms.

Under the hood, neuDrive exposes a canonical virtual tree together with typed APIs, file-tree access, and `snapshot/changes` sync interfaces.

Hosted service examples in this repo use:

- Hub URL: `https://www.neudrive.ai`
- MCP URL: `https://www.neudrive.ai/mcp`

## Start Here

Choose the first path that matches how you want to connect:

1. **Browser Extension**: Chrome / Edge sidecar for Claude, ChatGPT, Gemini, and Kimi with hosted login, context injection, and conversation import. [Open guide](docs/browser-extension.md)
2. **Web / Desktop Apps**: fastest path for Claude, ChatGPT, Cursor, and Windsurf through hosted neuDrive with browser auth. [Open guide](docs/setup.md#web-and-desktop-apps)
3. **CLI Apps**: Claude Code, Codex CLI, Gemini CLI, and Cursor Agent with remote HTTP MCP + OAuth. [Open guide](docs/setup.md#cli-apps)
4. **Local Mode**: repo-first local development, LAN setups, or any environment without a public HTTPS URL yet. [Open guide](docs/setup.md#local-mode)
5. **Advanced Mode / GPT Actions / Adapters**: generic HTTP MCP clients, custom GPTs, and webhook-style integrations such as Feishu. [Open guide](docs/setup.md#advanced-mode)

## Web and Desktop Apps

Use this when the connection starts from a graphical interface such as Claude web, ChatGPT, Cursor, or Windsurf.

If you want a lighter-weight sidecar experience inside existing chat pages instead of MCP / Apps, use the [Browser Extension Guide](docs/browser-extension.md).

### Claude Connectors

1. Sign in to the Claude web app and open `Settings -> Connectors -> Go to Customize`.
2. Click `Add custom connector`.
3. Set `Remote MCP Server URL` to `https://www.neudrive.ai/mcp`.
4. Save and click `Connect`.
5. Your browser will open the neuDrive sign-in and authorization flow; after approval, return to Claude.

### ChatGPT Apps

1. Sign in to ChatGPT and open `Settings -> Apps`.
2. In `Advanced settings`, click `Create app`.
3. Set `MCP Server URL` to `https://www.neudrive.ai/mcp`.
4. Follow the prompts to finish neuDrive sign-in and authorization.

If you do not see the `Apps` entry yet, your plan or rollout cohort probably does not have access to it yet.

After the connection is authorized, start a **new chat** and give it a direct import instruction such as:

- `Please import my skills, projects, and profile into neuDrive.`
- `Please read my neuDrive profile, skills, and recent project context, then summarize what is already there.`

For Cursor, Windsurf, and the full setup variants, see [Web / Desktop Apps](docs/setup.md#web-and-desktop-apps).

## Local CLI Quick Start

Install the CLI locally:

```bash
git clone https://github.com/agi-bar/neudrive.git
cd neudrive
./tools/install-neudrive.sh
```

After install, use `neu`; the `neudrive` compatibility alias still works.

```bash
neu status         # check daemon, storage, and current target readiness
neu platform ls    # list installed adapters and connection state
neu connect claude # install/configure the Claude integration
neu browse         # open the local Hub in your browser
```

Then open a **new chat** in the connected client and say something like `Please import this workspace's useful skills, project context, and profile/preferences into neuDrive.`

Detailed CLI usage: [CLI Guide](docs/cli.md)

## Login To Hosted Cloud

Use `neu login` when you want the hosted `official` profile backed by `https://www.neudrive.ai` for CLI work, hosted dashboard access, or cross-app sync flows.

```bash
neu login
```

This opens a browser login flow, saves the hosted `official` profile locally, and switches the current target to that profile.

## Documentation

Start here:

- [Browser Extension Guide](docs/browser-extension.md)
- [Setup Guide](docs/setup.md)
- [CLI Guide](docs/cli.md)
- [Reference](docs/reference.md)

Chinese docs:

- [Chinese README](README.zh-CN.md)
- [Chinese Browser Extension Guide](docs/browser-extension.zh-CN.md)
- [Chinese Setup Guide](docs/setup.zh-CN.md)
- [Chinese CLI Guide](docs/cli.zh-CN.md)
- [Chinese Reference](docs/reference.zh-CN.md)

More docs:

- [Token Management](docs/setup.md#token-management)
- [Bundle Sync guide](docs/sync.md)
- [SDK / HTTP API](docs/reference.md#sdk)
- [Product design document](docs/design.md)
- [Prod-like acceptance runbook](docs/sync-prodlike-acceptance.md)
- [Security and resource audit](docs/sync-audit.md)
- [CLI test matrix](docs/cli-test-matrix.md)
