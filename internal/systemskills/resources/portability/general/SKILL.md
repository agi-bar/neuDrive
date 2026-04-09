---
name: portability/general
description: Fallback guide for migrating data from platforms that do not yet have a dedicated AgentHub portability manual.
when_to_use: Use when the user asks to migrate, back up, restore, import, or export platform data and no dedicated portability/<platform> manual exists, or the dedicated manual does not cover the needed surface.
tags:
  - portability
  - migration
  - backup
  - general
  - agenthub
source: system
read_only: true
---
# General Platform Portability Manual

Use this manual when the source platform does not have its own `portability/<platform>` manual yet, or when the platform-specific manual exists but does not cover the exact asset type the user wants to migrate.

Treat Agent Hub as the canonical destination.
Preserve exact files and package structure when possible, and never silently collapse a richer platform structure into a thinner summary.

## First Decision

1. If a dedicated manual such as `portability/claude`, `portability/chatgpt`, or `portability/codex` exists and clearly covers the task, read that first.
2. Otherwise, fall back to this manual.
3. If the platform has some unique export surface but no dedicated manual, document that uniqueness explicitly instead of pretending it matches another platform.

## Generic Category Map

Classify the source platform into the nearest of these Agent Hub buckets before writing anything:

- `account-wide profile or preferences`
- `memory or durable facts`
- `projects or workspaces`
- `conversations or transcripts`
- `knowledge files or uploaded assets`
- `skills, reusable prompt bundles, or tool bundles`
- `tools, connectors, or integrations`
- `automations or scheduled behaviors`
- `official export packages`

Do not merge all of these into one "import everything" blob.

## Generic Interface Rules

- Use `update_profile` for stable, account-wide rules and preferences.
- Use `save_memory` for dated notes, extracted facts, and smaller derived memories.
- Use `create_project`, `get_project`, and `log_action` for project reconstruction when the imported data truly belongs to a project or workspace.
- Use `write_file` for any imported data that should be preserved as files, not just for projects. If an item does not fit a first-class Agent Hub domain such as profile, memory, project, or skill, the agent may still import it with `write_file` under a sensible custom directory structure.
- Prefer clear, self-describing paths when designing that structure, such as platform-scoped archive folders, manifests, knowledge mirrors, or export snapshots.
- Preserve unsupported structures as archive notes, structured metadata, or custom file trees instead of dropping them.

## Skill And Package Import Rules

Use these rules whenever the source platform has reusable prompt bundles, tool bundles, assistant packages, or anything that should land under Agent Hub `/skills`.

### `import_skill`

Use `import_skill(name, files)` when all of the following are true:

- this is one skill-like bundle
- every file can be represented as text/code in a `map[path]string`
- nested relative paths are enough to preserve the structure

Allowed examples include:

- `SKILL.md`
- `prompts/review.txt`
- `scripts/run.py`
- `scripts/build.sh`
- `config/tool.yaml`
- `data/schema.xsd`

Do not simplify the bundle to only `SKILL.md`.
If the skill directory includes prompts, scripts, config, schemas, or helper sources, include them too.

### `import_skills_archive`

Use `import_skills_archive` when any of these are true:

- there are multiple skills in one batch
- the bundle includes binary assets
- preserving exact bytes is important
- flattening everything into `map[path]string` would be lossy or tedious

Supported zip structures:

1. Single-skill archive at zip root:
   - `SKILL.md`
   - `prompts/...`
   - `scripts/...`
   - `assets/...`
   Agent Hub infers the skill name from the archive filename.
2. Multi-skill archive with top-level skill directories:
   - `skill-a/SKILL.md`
   - `skill-a/scripts/run.py`
   - `skill-b/SKILL.md`
   - `skill-b/assets/icon.png`
3. Every imported skill must include its own `SKILL.md`.

All imported skill files land under `/skills/<name>/...` in Agent Hub.
Do not ask the user to choose another destination path.

### `create_skills_import_token`

Use `create_skills_import_token` when a full archive is the right payload shape but one MCP tool call cannot carry the archive reliably.

This is the user-mediated fallback transport path:

1. Package one complete zip and keep the original directory structure.
2. Do not omit helper files such as `scripts/`, prompts, config, schemas, fonts, or assets.
3. Hand that zip to the user for download.
4. Call `create_skills_import_token`.
5. If the response includes a browser upload link, present it to ordinary users.
6. If the response includes a curl command, present it to terminal-comfortable users.
7. Mention both when both are available, then choose the recommended one based on user context.

Browser upload is the normal-user path.
Curl upload is the developer-friendly path.
Both still import into the Agent Hub `/skills` root.

## Generic Import Checklist

1. Inventory the source platform's categories and mark each one `available`, `missing`, or `blocked`.
2. Prefer exact exports or original files over reconstructed summaries.
3. Write stable preferences to profile first.
4. Rebuild project/workspace context second.
5. Preserve conversations, knowledge files, tools, automations, and other file-like assets as native Agent Hub data when possible, or with `write_file` under a clear custom directory structure when not.
6. For skill-like bundles, choose between `import_skill`, `import_skills_archive`, and `create_skills_import_token` by payload shape and transport limits.
7. Never import only `SKILL.md` when the skill depends on other files.
8. End with a report that lists `imported`, `archived`, and `blocked` items plus the exact interface used for each.

## Generic Export Checklist

1. Inventory Agent Hub data by category instead of by raw file path alone.
2. Rebuild the nearest platform-native structures without claiming unsupported parity.
3. If the target platform has no native skill format, generate one of:
   - reusable prompt bundles
   - setup instructions
   - archive manifests
4. Mark every manual follow-up step explicitly.

## Prompt Template

Use or adapt this prompt when another agent needs to execute portability work for an unsupported platform:

> Read `/skills/portability/general/SKILL.md` first. Inventory the source platform into account-wide preferences, memory, projects/workspaces, conversations, knowledge files, reusable skills or bundles, tools/connectors, automations, and official exports. Map each category to the nearest Agent Hub domain instead of collapsing everything into one summary. Use `update_profile` for stable rules, `save_memory` for smaller derived notes, `create_project` for true project/workspace reconstruction, and `write_file` for any additional imported data that should be preserved as files even when it does not fit a first-class Agent Hub domain. The agent may design a sensible custom directory structure for those files. Use `import_skill` only for one text/code skill directory whose full contents can be represented as strings, and `import_skills_archive` for multi-skill, binary-heavy, or exact-byte bundles. If one archive is too large for a single MCP tool call, switch to `create_skills_import_token`, hand the zip to the user for download, and tell the user to use either the returned browser upload link or the returned curl command. All skill imports land under the Agent Hub `/skills` root. Preserve unsupported structures as archive notes, structured metadata, or custom file trees instead of dropping them, and finish with imported, archived, and blocked items plus the exact interface used for each category.

{{CURRENT_USER_SNAPSHOT}}
