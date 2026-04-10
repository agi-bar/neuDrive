# `agenthub export`

Use this command when the user wants to export the current platform's data into Agent Hub.

## Goals

- Capture exact assets when they are directly available.
- Capture derived long-term memory, preferences, and working rules when only the agent can interpret them.
- Preserve unsupported or partially mapped content as archive metadata instead of dropping it.

## Steps

1. Read `/skills/agenthub/SKILL.md`.
2. Read `/skills/agenthub/references/platforms/<platform>.md` if present.
3. Read `/skills/portability/<platform>/SKILL.md` when a platform portability manual exists; otherwise read `/skills/portability/general/SKILL.md`.
4. Classify the source data by platform-native category before choosing tools.
5. Gather exact assets first.
6. Gather derived content second.
7. If the platform is Claude and the user already has the official Claude exported data zip, prefer the existing `/api/import/claude-data` path and note that public MCP parity does not exist yet.
8. If the platform is Claude and the user has Claude memory export text, prefer `/api/import/claude-memory` and note that public MCP parity does not exist yet.
9. If the task is one text/code skill whose files can be represented as strings, use `import_skill`. Nested paths like `scripts/run.py` are allowed, but still include the whole skill directory rather than only `SKILL.md`.
10. If the task is Claude Web skills under `/mnt/skills/user` or any multi-skill / binary-heavy archive, create one full zip and check the zip size before reading contents.
11. If the Claude Web zip is larger than `64 KB` or the size cannot be checked safely, do not read or base64 it into MCP args; use `prepare_skills_upload` plus `/agent/import/skills` instead.
12. Use `import_skills_archive` for Claude Web only when the zip is already known to be `<= 64 KB` and safe for one MCP tool call.
13. If the user still wants pure inline MCP transport, split by top-level skill directories only when each resulting zip is known to stay within the same `64 KB` limit.
14. All skill imports land under `/skills/<name>/...` in Agent Hub; a fallback upload flow should target the `/skills` root by default.
15. In that fallback flow, package a full zip with every file in the skill directories, hand the zip to the user for download, then present both the returned browser upload link and the curl command when available. Prefer the browser path for ordinary users and curl for terminal-comfortable users.
16. Write the result into Agent Hub through the chosen MCP or HTTP path, then report imported, archived, and blocked items explicitly.

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
