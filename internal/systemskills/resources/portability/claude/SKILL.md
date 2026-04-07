---
name: portability/claude
description: Guide for importing Claude data into AgentHub or restoring AgentHub data into Claude-compatible structures.
when_to_use: Use when the user asks to migrate, back up, restore, import, or export Claude data and skills.
tags:
  - portability
  - migration
  - backup
  - claude
  - agenthub
source: system
read_only: true
---
# Claude Portability Manual

## Overview

Use this skill when the user wants to move data between Claude and AgentHub.
Prefer native Claude import paths when available, especially existing exported data zip flows.

## When To Use

Use this skill for:

- backing up Claude memory, projects, skills, or exported data into AgentHub
- restoring AgentHub context back into Claude-compatible working materials
- explaining how Claude concepts map into AgentHub

## Platform Feature Map

- `Claude memory` -> `memory/profile/*`, `memory/scratch/*`, and Claude-specific archive notes when needed
- `Claude exported data zip` -> import into AgentHub through the existing `claude-data` flow when available
- `Claude Web skills zip` -> import into AgentHub `/skills/<name>/...` via the skills zip import path
- `Projects` -> `/projects/<name>/context.md` and related logs
- `.skill` directories -> `/skills/<name>/...`
- `Conversations` -> conversation archive and project references

## Import Into AgentHub

Recommended order:

1. If the user already has a Claude exported data zip, prefer that path first.
2. Import Claude memory and stable preferences before project-level details.
3. For Claude Web skills, prefer a zip export over piecemeal file copy. Ask Claude to package `/mnt/skills/user` into one zip, call `create_skills_import_token`, then upload that zip to AgentHub so the server can expand it into `/skills/<name>/...`.
4. Import `.skill` directories into AgentHub skills when a zip is not available.
5. Preserve conversation history and project documents as archive when they do not map cleanly.
6. End with a report describing native imports, archived content, and remaining manual work.

### Preferred Claude Web skills export flow

When the user wants all Claude Web skills in one step, prefer the built-in zip route:

```bash
cd /mnt/skills/user
zip -r /mnt/user-data/outputs/agenthub-skills.zip .
```

If the user wants a lighter export with only key files:

```bash
cd /mnt/skills/user
zip -r /mnt/user-data/outputs/agenthub-skills.zip . -i "*/SKILL.md" "*/*.py"
```

After the zip exists:

1. Call the Agent Hub MCP tool `create_skills_import_token` with a short purpose such as `claude-web-skills`.
2. Use the returned `upload_url` and `token` to upload the zip to Agent Hub's `/agent/import/skills` endpoint.

Example:

```bash
curl -f -X POST \
  -H "Authorization: Bearer <TOKEN>" \
  -F "platform=claude-web" \
  -F "file=@/mnt/user-data/outputs/agenthub-skills.zip" \
  "<UPLOAD_URL>"
```

AgentHub will unpack the archive and restore each skill under `/skills/<name>/...`.

## Export Back To Claude

When exporting AgentHub data back into Claude:

1. Convert AgentHub profile and principles into reusable working instructions.
2. Convert project context into Claude-ready context files or prompts.
3. Convert AgentHub skills into Claude-style skill directories when the structure is compatible.
4. Note that there is no dedicated Claude-specific restore pipeline yet; restoration is currently manual or prompt-driven.

## Known Limits

- Claude-native restore is still manual or documentation-driven.
- Exported conversation history may need summarization before reuse.
- Some Claude project structures may need archive preservation rather than first-class restore.
- Secrets should remain in AgentHub vault rather than leaving the Hub by default.

## Prompt Template

Use or adapt this prompt when another agent needs to execute Claude portability work:

> Read `/skills/portability/claude/SKILL.md` first. If a Claude exported data zip exists, use it as the primary import source. Map Claude memory, projects, skills, and conversations into AgentHub. Preserve anything that does not map cleanly as archive instead of dropping it. When exporting back to Claude, generate manual restoration instructions and compatible skill materials where possible.

{{CURRENT_USER_SNAPSHOT}}
