---
name: portability/chatgpt
description: Guide for importing ChatGPT data into AgentHub or restoring AgentHub data into ChatGPT-compatible structures.
when_to_use: Use when the user asks to migrate, back up, restore, import, or export ChatGPT data and platform features.
tags:
  - portability
  - migration
  - backup
  - chatgpt
  - agenthub
source: system
read_only: true
---
# ChatGPT Portability Manual

## Overview

Use this skill when the user wants to move data between ChatGPT and AgentHub.
Treat AgentHub as the canonical store, preserve original meaning, and never hide portability gaps.

## When To Use

Use this skill for:

- backing up ChatGPT data into AgentHub
- restoring AgentHub data into ChatGPT-compatible structures
- mapping ChatGPT features into AgentHub domains
- producing a step-by-step migration prompt for another agent

## Platform Feature Map

- `Custom Instructions` -> `memory/profile/preferences.md` and `memory/profile/principles.md`
- `Saved Memory` -> `memory/profile/*` for stable facts, `memory/scratch/*` for transient context
- `Projects` -> `/projects/<name>/context.md` plus project logs
- `Chats / conversation history` -> archived conversation assets
- `Library / Knowledge uploads` -> knowledge and file assets
- `Custom GPT configuration` -> tool and connection shadow metadata
- `GPT Actions` -> connection and tool metadata, not live secrets
- `Connectors / integrations` -> connection metadata plus vault references
- `Automations / scheduled behaviors` -> automation shadow records and recreation notes

## Skill And Package Rules

ChatGPT does not expose Claude-style `/mnt/skills/user` directories.
If the migration still needs reusable prompt/code/tool bundles to land under Agent Hub `/skills`, the agent must assemble that bundle explicitly.

- Use `import_skill(name, files)` for one text/code bundle whose full directory can be represented as `map[path]string`.
- Nested paths like `scripts/run.py`, `prompts/review.txt`, and `config/tool.yaml` are allowed.
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

1. Identify whether the user wants profile, memory, projects, knowledge files, conversations, tools, connections, automations, or everything.
2. Classify each item into AgentHub domains before writing.
3. Write stable rules and preferences into `memory/profile`.
4. Write project context into `/projects/<name>/context.md`.
5. Use `write_file` for imported data that should be preserved as files even when it does not fit a first-class Agent Hub domain such as profile, memory, project, or skill. The agent may design a sensible custom directory structure for those files.
6. If the task includes reusable bundles that should live under Agent Hub `/skills`, apply the skill/package rules above and preserve the entire directory contents, not just the instruction file.
7. Preserve chats, knowledge uploads, GPT configuration, and other unsupported surfaces as structured archive, shadow metadata, or custom file trees when no first-class domain exists yet.
8. End with a coverage report: native imports, archived items, manual follow-ups, and unsupported parity.

## Export Back To ChatGPT

When exporting AgentHub data back into ChatGPT:

1. Compress stable preferences into reusable Custom Instructions text.
2. Convert project context into one project seed document per project.
3. Prepare a knowledge upload manifest for files and references.
4. Generate draft GPT Actions configuration from stored tool metadata.
5. Mark manual recreation steps explicitly when a ChatGPT-native feature has no direct automated restore path.

## Known Limits

- ChatGPT feature availability may vary by account and product surface.
- Knowledge uploads and library-like assets may require manual handling.
- GPT Actions can usually be preserved as metadata and draft configuration, but not always auto-restored.
- Secrets and tokens should stay governed by AgentHub vault policy by default.
- Automation parity is partial and should be described as intent plus recreation guidance.

## Prompt Template

Use or adapt this prompt when another agent needs to execute ChatGPT portability work:

> Help me migrate data between ChatGPT and AgentHub. First classify the data into profile, memory, projects, knowledge/files, tools/connections, automations, conversations, and any reusable bundles that should live under Agent Hub `/skills`. Then map each item to the nearest AgentHub canonical domain. Use `write_file` for additional imported file-like data that should be preserved even when it does not fit a first-class Agent Hub domain, and choose a sensible custom directory structure for those files. If one bundle is a text/code skill-like directory, use `import_skill` with the full directory contents, not just `SKILL.md`. If the user asks for all skills, a workspace export, or any multi-bundle batch, do not use `import_skill` as the primary transport. If the bundle is multi-skill, binary-heavy, or too large for one tool call, use `import_skills_archive` or `prepare_skills_upload` as appropriate. Do not `cat` base64(zip), paste archive base64 into chat, or otherwise emit long archive strings into the conversation. Preserve ChatGPT-specific structure as archive, shadow metadata, or custom file trees instead of dropping it. If exporting back to ChatGPT, generate the nearest ChatGPT-compatible outputs and clearly mark manual steps and unsupported parity.

{{CURRENT_USER_SNAPSHOT}}
