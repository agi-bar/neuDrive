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

## Import Into AgentHub

Recommended order:

1. Identify whether the user wants profile, memory, projects, knowledge files, conversations, tools, connections, automations, or everything.
2. Classify each item into AgentHub domains before writing.
3. Write stable rules and preferences into `memory/profile`.
4. Write project context into `/projects/<name>/context.md`.
5. Preserve chats, knowledge uploads, and GPT configuration as structured archive or shadow metadata when no first-class domain exists yet.
6. End with a coverage report: native imports, archived items, manual follow-ups, and unsupported parity.

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

> Help me migrate data between ChatGPT and AgentHub. First classify the data into profile, memory, projects, knowledge/files, tools/connections, automations, and conversations. Then map each item to the nearest AgentHub canonical domain. Preserve ChatGPT-specific structure as archive or shadow metadata instead of dropping it. If exporting back to ChatGPT, generate the nearest ChatGPT-compatible outputs and clearly mark manual steps and unsupported parity.

{{CURRENT_USER_SNAPSHOT}}
