# Claude Entry

- Local entrypoint: `/agenthub <subcommand>`
- Installed local skill: `agenthub`
- Before migrating, classify Claude data into: `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`, and `official exports`.
- Use `export` to capture Claude-visible data into Agent Hub.
- If the user already has the official Claude exported data zip, prefer `/api/import/claude-data`. Public MCP parity for that full export path does not exist yet.
- If the user has Claude memory export text, prefer `/api/import/claude-memory`. Public MCP parity for that path also does not exist yet.
- For one text/code skill whose files can be represented as strings, use `import_skill`. Nested paths like `scripts/run.py` are allowed, but still include the whole skill directory rather than only `SKILL.md`.
- For Claude Web skills under `/mnt/skills/user` or any multi-skill / binary-heavy zip, prefer one full archive and call `import_skills_archive`.
- If a skills archive is too large, split it by top-level skill directories and import multiple batches.
- If one MCP tool call cannot carry the archive reliably, use `create_skills_import_token` plus `/agent/import/skills` as the user-mediated fallback transport path, including inside Claude Web.
- All skill imports land under `/skills/<name>/...`; a fallback upload flow should target the `/skills` root by default.
- In that fallback flow, have Claude package a full zip with every file in the skill directories, give the zip to the user for download, then present both the returned browser upload link and the curl command when available. Prefer the browser path for ordinary users and curl for terminal-comfortable users.
- Use `import` to generate Claude-compatible materials from Agent Hub.
- Use `list` to inspect supported domains and discovered sources.
- Use `status` to verify MCP, command install, and daemon readiness.
