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
4. Do not choose `import_skill`, `import_skills_archive`, or `prepare_skills_upload` until that portability manual has been read.
5. Classify the source data by platform-native category before choosing tools.
6. Gather exact assets first.
7. Gather derived content second.
8. If the platform is Claude and the user already has the official Claude exported data zip, prefer the existing `/api/import/claude-data` path and note that public MCP parity does not exist yet.
9. If the platform is Claude and the user has Claude memory export text, prefer `/api/import/claude-memory` and note that public MCP parity does not exist yet.
10. Use `import_skill` only when the task is one complete text/code skill whose files can be represented as strings. Nested paths like `scripts/run.py` are allowed, but still include the whole skill directory rather than only `SKILL.md`.
11. If the user asks for "all skills", a workspace export, `/mnt/skills/user`, or any multi-skill / binary-heavy archive, do not choose `import_skill`; create one full zip and check the zip size before reading contents.
12. If the Claude Web zip is larger than `64 KB` or the size cannot be checked safely, do not read or base64 it into MCP args; use `prepare_skills_upload` plus `/agent/import/skills` instead.
13. After `prepare_skills_upload`, first POST the returned `connectivity_probe_url`. If it returns `200`, use the returned `curl_example` to upload directly from the Claude sandbox.
14. If the probe fails in Claude Web, tell the user to open `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains`, add the Agent Hub host such as `agenthub.agi.bar`, and retry the direct upload.
15. If the user already added the domain and the current Claude Web conversation still fails the probe, explain that the new setting may require a new conversation before it takes effect, and ask whether they want to start a new conversation and retry or use the fallback now.
16. If the user does not add the domain, does not want to start a new conversation, or direct upload is still blocked, use the returned browser upload link for ordinary users or the returned curl command for terminal-comfortable users.
17. Use `import_skills_archive` for Claude Web only when the zip is already known to be `<= 64 KB` and safe for one MCP tool call.
18. If the user still wants pure inline MCP transport, split by top-level skill directories only when each resulting zip is known to stay within the same `64 KB` limit.
19. All skill imports land under `/skills/<name>/...` in Agent Hub; a fallback upload flow should target the `/skills` root by default.
20. Write the result into Agent Hub through the chosen MCP or HTTP path, then report imported, archived, and blocked items explicitly.

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
