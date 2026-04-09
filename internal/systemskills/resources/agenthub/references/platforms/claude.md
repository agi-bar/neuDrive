# Claude Entry

- Local entrypoint: `/agenthub <subcommand>`
- Installed local skill: `agenthub`
- Before migrating, classify Claude data into: `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`, and `official exports`.
- Use `export` to capture Claude-visible data into Agent Hub.
- If the user already has the official Claude exported data zip, prefer `/api/import/claude-data`. Public MCP parity for that full export path does not exist yet.
- If the user has Claude memory export text, prefer `/api/import/claude-memory`. Public MCP parity for that path also does not exist yet.
- For one skill, use `import_skill`.
- For Claude Web skills under `/mnt/skills/user` or any multi-skill zip, prefer one full archive and call `import_skills_archive`.
- If a skills archive is too large, split it by top-level skill directories and import multiple batches.
- Use `create_skills_import_token` plus `/agent/import/skills` only as the fallback transport path outside Claude Web, or when direct MCP archive transfer is unavailable and the environment can actually make outbound HTTP requests.
- Use `import` to generate Claude-compatible materials from Agent Hub.
- Use `list` to inspect supported domains and discovered sources.
- Use `status` to verify MCP, command install, and daemon readiness.
