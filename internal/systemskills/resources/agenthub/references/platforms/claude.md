# Claude Entry

- Local entrypoint: `/agenthub <subcommand>`
- Installed local skill: `agenthub`
- Use `export` to capture Claude-visible data into Agent Hub.
- For Claude Web skills, prefer exporting `/mnt/skills/user` as one zip, calling `create_skills_import_token`, and uploading that zip into Agent Hub. This keeps full skill directories intact under `/skills/<name>/...`.
- Use `import` to generate Claude-compatible materials from Agent Hub.
- Use `list` to inspect supported domains and discovered sources.
- Use `status` to verify MCP, command install, and daemon readiness.
