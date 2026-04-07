---
name: agenthub
description: Use Agent Hub as the canonical hub for platform import, export, listing, and status workflows through MCP plus platform-native entrypoints.
tags:
  - agenthub
  - mcp
  - sync
  - portability
---

# Agent Hub

Use this umbrella skill when the user wants to work with Agent Hub from inside a supported platform such as Codex or Claude.

## Core Model

- Agent Hub MCP is the full capability surface.
- This local `agenthub` skill is the entry layer that routes platform-native commands to the right MCP tools and local platform actions.
- Treat Agent Hub as the canonical destination for imported data.

## Commands

- `export`
  Read `/skills/agenthub/commands/export.md`
- `import`
  Read `/skills/agenthub/commands/import.md`
- `list`
  Read `/skills/agenthub/commands/list.md`
- `status`
  Read `/skills/agenthub/commands/status.md`
- `help`
  Read `/skills/agenthub/commands/help.md`

## Rules

- Use Agent Hub MCP tools for Hub reads and writes instead of inventing local file formats.
- Preserve exact assets separately from derived summaries.
- When a platform-specific portability manual exists, read `/skills/portability/<platform>/SKILL.md` before migrating data.
- Never silently drop unsupported or partially captured data; preserve it as notes or archive metadata.

