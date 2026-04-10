---
name: portability/codex
description: Guide for importing Codex workspace conventions into AgentHub or exporting AgentHub context back into Codex workflows.
when_to_use: Use when the user asks to migrate, back up, restore, import, or export Codex projects, prompts, tools, or automations.
tags:
  - portability
  - migration
  - backup
  - codex
  - agenthub
source: system
read_only: true
---
# Codex Portability Manual

## Overview

Use this skill when the user wants to move data between Codex and AgentHub.
Codex portability is manual-first: preserve workspace structure, conventions, prompts, tool configuration, and automation intent without claiming full feature parity.

## When To Use

Use this skill for:

- backing up Codex workspace context into AgentHub
- restoring AgentHub context into Codex-compatible prompts, files, or setup instructions
- mapping Codex project instructions, tools, and automations into AgentHub

## Platform Feature Map

- `workspace or project instructions` -> `/projects/<name>/context.md` plus profile rules where stable
- `reusable prompts` -> skill-like reference material or project assets
- `tools / MCP config` -> tool metadata and connection metadata
- `automation manifests` -> automation shadow records
- `session transcripts or notes` -> conversation archive and project logs

## Skill And Package Rules

Codex portability often produces reusable prompt bundles, scripts, or helper packages that should live under Agent Hub `/skills`.

- Use `import_skill(name, files)` for one text/code bundle whose full directory can be represented as `map[path]string`.
- Nested paths like `scripts/run.py`, `prompts/review.txt`, `config/tool.yaml`, and `data/schema.xsd` are allowed.
- Do not simplify a bundle to only `SKILL.md`; include the whole bundle directory and every text/code file it depends on.
- If the user asks for all skills, a workspace export, or any multi-bundle batch, do not use `import_skill` as the primary transport.
- Use `import_skills_archive` for multi-bundle imports, binary-heavy bundles, or any case where exact bytes matter.
- Supported zip layouts are:
  - one skill at zip root: `SKILL.md`, `scripts/...`, `prompts/...`, `assets/...`
  - many skills as top-level directories: `skill-a/SKILL.md`, `skill-b/SKILL.md`, and related files below each directory
- Every imported skill directory must contain `SKILL.md`.
- All skill imports land under Agent Hub `/skills/<name>/...`.
- Do not `cat` base64(zip), paste archive base64 into chat, or otherwise emit long archive strings into the conversation, because that can crash the conversation session.
- If one archive is too large for a single MCP tool call, use `prepare_skills_upload` and present both the browser upload link and the curl command when available. Prefer the browser path for ordinary users and curl for terminal-comfortable users.

## Import Into AgentHub

Recommended order:

1. Classify stable workspace conventions versus project-specific instructions.
2. Write stable preferences into `memory/profile`.
3. Write project-specific context into `/projects/<name>/context.md`.
4. Use `write_file` for imported data that should be preserved as files even when it does not fit a first-class Agent Hub domain such as profile, memory, project, or skill. The agent may design a sensible custom directory structure for those files.
5. If reusable prompt/script bundles should live under Agent Hub `/skills`, apply the skill/package rules above and preserve the full directory contents.
6. Preserve tool and MCP configuration as structured metadata.
7. Preserve automation manifests as intent plus schedule notes.
8. Preserve transcripts, outputs, and other unsupported file-like assets with archive notes, metadata, or custom file trees when no first-class domain exists yet.

## Export Back To Codex

When exporting AgentHub data back into Codex:

1. Generate workspace or project instruction files from AgentHub project context.
2. Generate reusable prompt bundles from profile and skill content.
3. Generate draft tool and MCP configuration notes from stored metadata.
4. Generate automation recreation notes from stored automation intent.
5. Keep the process manual-first and mark every assumption clearly.

## Known Limits

- Codex portability currently relies on manual or prompt-driven reconstruction.
- There is no dedicated Codex-native import/export pipeline yet.
- Tool and MCP configuration can be preserved as metadata, but live credentials should stay in AgentHub vault.
- Automation parity is documentation-first.

## Prompt Template

Use or adapt this prompt when another agent needs to execute Codex portability work:

> Read `/skills/portability/codex/SKILL.md` first. Separate stable workspace conventions from project-specific context. Write stable rules into AgentHub profile, write true project context into AgentHub projects, use `write_file` for additional imported file-like data that should be preserved even when it does not fit a first-class Agent Hub domain, and preserve tool and MCP configuration as metadata plus automation intent as recreation notes. If reusable prompt or script bundles should live under Agent Hub `/skills`, use `import_skill` for one text/code directory with full contents. If the user asks for all skills, a workspace export, or any multi-bundle batch, do not use `import_skill` as the primary transport; use `import_skills_archive` / `prepare_skills_upload` for multi-skill, binary-heavy, or transport-limited archives instead. Do not `cat` base64(zip), paste archive base64 into chat, or otherwise emit long archive strings into the conversation. When exporting back to Codex, produce manual-first setup instructions and clearly mark unsupported parity.

{{CURRENT_USER_SNAPSHOT}}
