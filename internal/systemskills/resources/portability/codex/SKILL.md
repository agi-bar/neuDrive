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

## Import Into AgentHub

Recommended order:

1. Classify stable workspace conventions versus project-specific instructions.
2. Write stable preferences into `memory/profile`.
3. Write project-specific context into `/projects/<name>/context.md`.
4. Preserve tool and MCP configuration as structured metadata.
5. Preserve automation manifests as intent plus schedule notes.
6. Preserve transcripts and outputs as archive when no first-class domain exists yet.

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

> Read `/skills/portability/codex/SKILL.md` first. Separate stable workspace conventions from project-specific context. Write stable rules into AgentHub profile, write project context into AgentHub projects, preserve tool and MCP configuration as metadata, and preserve automation intent as recreation notes. When exporting back to Codex, produce manual-first setup instructions and clearly mark unsupported parity.

{{CURRENT_USER_SNAPSHOT}}
