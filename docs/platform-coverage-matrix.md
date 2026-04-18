# Platform Coverage Matrix

This matrix tracks the value-based local import surface for `Claude` and `Codex`.

- `files` mode stays a curated raw tree snapshot into `/platforms/<platform>/...`.
- `files` mode does not zip the whole home directory.
- Default import intent is durable, user-meaningful, reconstructable data.
- Low-value runtime noise is excluded from default discovery and default imports.
- `Codex` rows describe the local layout observed by neuDrive. They do not claim an official on-disk vendor spec.

Status vocabulary:

- `first-class`
- `conversation archive`
- `structured metadata`
- `exact snapshot`
- `excluded`

| Platform | Category ID | Current | Target | Source roots / importer | Notes |
| --- | --- | --- | --- | --- | --- |
| Claude | `claude.profile-rules` | `first-class` | `first-class` | `~/.claude/CLAUDE.md`, `~/.claude/CLAUDE.local.md`, `~/.claude/output-styles/` | Output styles are promoted into profile/style rules. |
| Claude | `claude.memory` | `first-class` | `first-class` | `~/.claude/agent-memory/`, `~/.claude/memory/`, `~/.claude/projects/*/memory/` | Durable memory only. |
| Claude | `claude.projects` | `first-class` | `first-class` | `~/.claude.json`, workspace roots from discovered projects | Project context plus knowledge files. |
| Claude | `claude.bundles` | `first-class` | `first-class` | `~/.claude/skills/`, `~/.claude/agents/`, `~/.claude/commands/`, `~/.claude/rules/` | Skills, agents, commands, and rules bundles. |
| Claude | `claude.conversations` | `conversation archive` | `conversation archive` | `~/.claude/projects/**/*.jsonl` | Canonical archive under `/conversations/claude-code/...`. |
| Claude | `claude.automations` | `structured metadata` + `exact snapshot` | `structured metadata` + `exact snapshot` | `~/.claude/scheduled-tasks/` | Parsed automation metadata plus raw scheduled-task files. |
| Claude | `claude.tools` | `structured metadata` | `structured metadata` | `~/.claude/plugins/installed_plugins.json`, plugin manifests under `~/.claude/plugins/` | Agent mode keeps metadata only; files mode keeps raw tree when present. |
| Claude | `claude.connections` | `structured metadata` | `structured metadata` | `~/.claude.json`, `~/.claude/settings.json`, `~/.claude/settings.local.json`, project `.mcp.json` and `.claude/settings*.json` | Connection/config manifests are summarized, not re-imported as first-class files. |
| Claude | `claude.history` | `exact snapshot` | `exact snapshot` | `~/.claude/history.jsonl` | Retained as raw session evidence. |
| Claude | `claude.hooks` | `exact snapshot` | `exact snapshot` | `~/.claude/hooks/` | Preserved as file tree only. |
| Claude | `claude.official-export-zip` | dedicated importer | dedicated importer | `/api/import/claude-data` | Official exported data zip stays on the dedicated HTTP path. |
| Claude | `claude.official-memory-export` | dedicated importer | dedicated importer | `/api/import/claude-memory` | Claude memory export text stays on the dedicated HTTP path. |
| Claude | `claude.excluded.todos` | `excluded` | `excluded` | `~/.claude/todos/` | Low-value runtime scratch. |
| Claude | `claude.excluded.plans` | `excluded` | `excluded` | `~/.claude/plans/` | Low-value planning scratch. |
| Claude | `claude.excluded.channels` | `excluded` | `excluded` | `~/.claude/channels/` | Low-value runtime channel state. |
| Claude | `claude.excluded.credentials-file` | `excluded` | `excluded` | `~/.claude/.credentials.json` | Only sensitive-finding and vault-candidate reporting is retained. |
| Codex | `codex.profile-rules` | `first-class` | `first-class` | `~/.codex/AGENTS.md`, `~/.codex/rules/`, `~/.codex/config.toml` | `config.toml` also contributes structured runtime preferences. |
| Codex | `codex.memory` | `first-class` | `first-class` | `~/.codex/memories/` | Durable memory only. |
| Codex | `codex.projects` | `first-class` | `first-class` | `~/.codex/sessions/`, `~/.codex/archived_sessions/`, `~/.codex/session_index.jsonl` | Workspace/project summaries are derived from observed session inventory. |
| Codex | `codex.conversations` | `conversation archive` | `conversation archive` | `~/.codex/sessions/**/*.jsonl`, `~/.codex/archived_sessions/**/*.jsonl` | Canonical archive under `/conversations/codex/...`. |
| Codex | `codex.connections` | `structured metadata` | `structured metadata` | `~/.codex/config.toml`, `~/.codex/auth.json` | MCP and auth session metadata only; secret values are not imported. |
| Codex | `codex.skills.user` | `first-class` | `first-class` | `~/.agents/skills/` | User-authored skills become `/skills/<name>/...`. |
| Codex | `codex.skills.bundled` | `first-class` | `first-class` | `~/.codex/skills/` | Bundled Codex skills become `/skills/codex-bundled-<name>/...`. |
| Codex | `codex.automations` | `structured metadata` + `exact snapshot` | `structured metadata` + `exact snapshot` | `~/.codex/automations/` | Parsed TOML metadata plus raw files in `files` mode. |
| Codex | `codex.tools.plugins` | `structured metadata` | `structured metadata` | `~/.codex/.tmp/plugins/plugins/*/.codex-plugin/plugin.json` | Agent mode keeps manifest metadata only. |
| Codex | `codex.history` | `exact snapshot` | `exact snapshot` | `~/.codex/history.jsonl` | Retained as raw activity evidence. |
| Codex | `codex.excluded.logs-db` | `excluded` | `excluded` | `~/.codex/logs_2.sqlite` | Internal runtime state only. |
| Codex | `codex.excluded.state-db` | `excluded` | `excluded` | `~/.codex/state_5.sqlite` | Internal runtime state only. |
| Codex | `codex.excluded.shell-snapshots` | `excluded` | `excluded` | `~/.codex/shell_snapshots/` | Temporary execution snapshots. |
| Codex | `codex.excluded.worktrees` | `excluded` | `excluded` | `~/.codex/worktrees/` | Temporary execution state. |
| Codex | `codex.excluded.global-state` | `excluded` | `excluded` | `~/.codex/.codex-global-state.json` | Internal runtime state only. |
| Codex | `codex.excluded.plugin-cache-assets` | `excluded` | `excluded` | `~/.codex/.tmp/plugins/` except manifest `plugin.json` | Only manifest metadata is retained. |
