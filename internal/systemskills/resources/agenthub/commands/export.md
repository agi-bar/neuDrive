# `agenthub export`

Use this command when the user wants to export the current platform's data into Agent Hub.

## Goals

- Capture exact assets when they are directly available.
- Capture derived long-term memory, preferences, and working rules when only the agent can interpret them.
- Preserve unsupported or partially mapped content as archive metadata instead of dropping it.

## Steps

1. Read `/skills/agenthub/SKILL.md`.
2. Read `/skills/agenthub/references/platforms/<platform>.md` if present.
3. Read `/skills/portability/<platform>/SKILL.md` when a platform portability manual exists.
4. Gather exact assets first.
5. Gather derived content second.
6. If the platform is Claude Web and skills live under `/mnt/skills/user`, prefer packaging them into one zip, call `create_skills_import_token`, then upload the zip to Agent Hub's skills import path.
7. Write the result into Agent Hub through MCP or the shared shell orchestrator.

## Output Shape

Produce or consume structured export data containing:

- `profile_rules`
- `memory_items`
- `projects`
- `automations`
- `tools`
- `connections`
- `archives`
- `unsupported`
- `notes`

Every derived item must include provenance such as source platform, capture mode, and exactness.
