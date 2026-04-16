---
name: neudrive
description: Use neuDrive as the canonical hub for platform import, export, listing, and status workflows through MCP plus platform-native entrypoints.
tags:
  - neudrive
  - mcp
  - sync
  - portability
---

# neuDrive

Use this umbrella skill when the user wants to work with neuDrive from inside a supported platform such as Codex or Claude.

## Core Model

- neuDrive MCP is the supported public capability surface for current product workflows.
- This local `neudrive` skill is the entry layer that routes platform-native commands to the right MCP tools and local platform actions.
- Treat neuDrive as the canonical destination for imported data.
- Current public surface focuses on profile, memory, projects, skills, tree, token, and sync workflows.
- Roles, inbox, and collaboration remain deferred product concepts. Do not treat them as currently supported public neuDrive tools.

## Commands

- `ls`
  Read `/skills/neudrive/commands/ls.md`
- `read`
  Read `/skills/neudrive/commands/read.md`
- `write`
  Read `/skills/neudrive/commands/write.md`
- `search`
  Read `/skills/neudrive/commands/search.md`
- `create`
  Read `/skills/neudrive/commands/create.md`
- `log`
  Read `/skills/neudrive/commands/log.md`
- `import`
  Read `/skills/neudrive/commands/import.md`
- `token`
  Read `/skills/neudrive/commands/token.md`
- `stats`
  Read `/skills/neudrive/commands/stats.md`
- `export`
  Read `/skills/neudrive/commands/export.md`
- `status`
  Read `/skills/neudrive/commands/status.md`
- `help`
  Read `/skills/neudrive/commands/help.md`

## Quick Start

- Codex examples: `$neudrive help`, `$neudrive ls`, `$neudrive read profile/preferences`, `$neudrive status`
- Claude examples: `/neudrive help`, `/neudrive ls`, `/neudrive read profile/preferences`, `/neudrive status`

## Rules

- Use neuDrive MCP tools for Hub reads and writes instead of inventing local file formats.
- Preserve exact assets separately from derived summaries.
- When a platform-specific portability manual exists, read `/skills/portability/<platform>/SKILL.md` before migrating data or choosing import/export tools.
- When no platform-specific manual exists, fall back to `/skills/portability/general/SKILL.md`.
- For skills migration, especially "all skills", workspace exports, or zip-based imports, do not choose `import_skill`, `import_skills_archive`, or `prepare_skills_upload` until the portability manual has been read.
- Never silently drop unsupported or partially captured data; preserve it as notes or archive metadata.
