# Claude Portability Examples

## Import Prompt

> Read `/skills/portability/claude/SKILL.md` first. Inventory the Claude-side categories as `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`, and `official exports`. Handle each category separately instead of merging them into one migration step. Use `update_profile` for durable account-wide rules, `save_memory` for smaller derived notes, `create_project` plus `write_file` for project reconstruction, and `import_skill` only for one complete text/code skill directory. If the user asks for all skills, `/mnt/skills/user`, `public + examples`, or any other multi-skill workspace, do not use `import_skill`; package the full directories first. Never reduce a skill import to only `SKILL.md`; keep scripts, prompts, config, schemas, and other dependencies together. For Claude Web multi-skill archives, first check zip size before any base64 read. If the Claude Web zip is larger than `64 KB` or size is unknown, call `prepare_skills_upload` instead of `import_skills_archive`. After that, POST the returned `connectivity_probe_url`; if it returns `200`, use the returned `curl_example` from the Claude sandbox. If the POST fails in Claude Web, tell the user to add the Agent Hub host to `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains`, then retry. If the user already has the official Claude export zip, note that `/api/import/claude-data` is the preferred path and that there is no public MCP equivalent yet. Preserve unsupported material as archive notes instead of dropping it.

## Export Prompt

> Read `/skills/portability/claude/SKILL.md` first. Inventory Agent Hub data by Claude category: `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, and `connectors`. Rebuild each category into the nearest Claude-native surface. Rebuild one skill directly from `/skills/<name>/...`; for many skills or asset-heavy skills, prefer a full archive workflow rather than cherry-picking files. Mark every manual restore step and every missing parity item explicitly.

## Reporting Template

End with:

- Summary
- Category inventory
- Imported items
- Archived items
- Blocked items
- Interfaces used
- Manual follow-ups
- Unsupported or unknown parity
