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

- Agent Hub MCP is the supported public capability surface for current product workflows.
- This local `agenthub` skill is the entry layer that routes platform-native commands to the right MCP tools and local platform actions.
- Treat Agent Hub as the canonical destination for imported data.
- Current public surface focuses on profile, memory, projects, skills, tree, token, and sync workflows.
- Devices, roles, inbox, and collaboration remain deferred product concepts. Do not treat them as currently supported public Agent Hub tools.

## Commands

- `ls`
  Read `/skills/agenthub/commands/ls.md`
- `read`
  Read `/skills/agenthub/commands/read.md`
- `write`
  Read `/skills/agenthub/commands/write.md`
- `search`
  Read `/skills/agenthub/commands/search.md`
- `create`
  Read `/skills/agenthub/commands/create.md`
- `log`
  Read `/skills/agenthub/commands/log.md`
- `import`
  Read `/skills/agenthub/commands/import.md`
- `token`
  Read `/skills/agenthub/commands/token.md`
- `stats`
  Read `/skills/agenthub/commands/stats.md`
- `git`
  Read `/skills/agenthub/commands/git.md`
- `export`
  Read `/skills/agenthub/commands/export.md`
- `status`
  Read `/skills/agenthub/commands/status.md`
- `help`
  Read `/skills/agenthub/commands/help.md`

## Quick Start

- Codex examples: `$agenthub help`, `$agenthub ls`, `$agenthub read profile/preferences`, `$agenthub git init`
- Claude examples: `/agenthub help`, `/agenthub ls`, `/agenthub read profile/preferences`, `/agenthub git init`

## Rules

- Use Agent Hub MCP tools for Hub reads and writes instead of inventing local file formats.
- Preserve exact assets separately from derived summaries.
- When a platform-specific portability manual exists, read `/skills/portability/<platform>/SKILL.md` before migrating data or choosing import/export tools.
- When no platform-specific manual exists, fall back to `/skills/portability/general/SKILL.md`.
- For skills migration, especially "all skills", workspace exports, or zip-based imports, do not choose `import_skill`, `import_skills_archive`, or `prepare_skills_upload` until the portability manual has been read.
- Never silently drop unsupported or partially captured data; preserve it as notes or archive metadata.
