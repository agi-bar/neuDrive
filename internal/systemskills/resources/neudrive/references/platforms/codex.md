# Codex Entry

- Local entrypoint: `$neudrive <subcommand>`
- Alternative discovery path: `/skills` and select `neudrive`
- Use `export` to capture Codex-visible data into neuDrive.
- Use `import` to restore neuDrive material back into Codex-compatible files and notes.
- Use `list` to inspect supported domains and discovered sources.
- Use `status` to verify MCP, skill install, and daemon readiness.

# Codex Local Layout Map

Use this map as the default source of truth for local Codex preview and import. Prefer deterministic path-based classification before attempting any live semantic scan.

## Fast Path Rules

- Start from known roots under `~/.codex` and `~/.agents`.
- Classify by directory and filename first.
- Open file contents only when the path is known but the import payload needs structured fields from the file.
- Preserve unsupported or partially mapped data as archived artifacts or notes instead of dropping it.
- Local Codex preview/import should default to this deterministic mapping; live `codex exec` semantic export is optional and should not block the core flow.

## Known Source Map

| Local path | Meaning | neuDrive target |
| --- | --- | --- |
| `~/.codex/AGENTS.md` | Global working rules | `profile_rules` |
| `~/.codex/rules/**/*.{rules,md,txt}` | Additional reusable rules | `profile_rules` |
| `~/.codex/memories/**/*.{md,txt}` | Durable memory notes | `memory_items` |
| `~/.codex/config.toml` | Runtime defaults, trusted projects, MCP server config | derived `profile_rules` plus `connections` metadata |
| `~/.codex/auth.json` | Local auth session metadata | `connections` metadata plus sensitive findings and vault candidates |
| `~/.codex/session_index.jsonl` | Session discovery index | project/session inventory input |
| `~/.codex/sessions/**` | Active session transcripts and workspace evidence | `projects` and archived artifacts |
| `~/.codex/archived_sessions/**` | Archived session transcripts | `projects` and archived artifacts |
| `~/.codex/automations/**` | Automation manifests and notes | archived `automations` records |
| `~/.agents/skills/**` | User-installed Codex skills | archived skill inventory |
| `~/.codex/skills/**` | Bundled/runtime skills | archived skill inventory |

## Import Guidance

- Trusted workspace paths found in `config.toml` and session metadata should be grouped into project context records.
- MCP server env keys and auth token presence should be treated as sensitive metadata; never import secret values in plaintext.
- Unknown files inside known roots should still be preserved via raw snapshot or archive notes.
- Unknown roots outside the map should not slow down the default import path; treat them as optional follow-up discovery work.
