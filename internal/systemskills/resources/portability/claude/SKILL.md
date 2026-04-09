---
name: portability/claude
description: Checklist-first guide for importing Claude data into AgentHub and exporting AgentHub context back into Claude-compatible materials.
when_to_use: Use when the user asks to migrate, back up, restore, import, or export Claude data and skills.
tags:
  - portability
  - migration
  - backup
  - claude
  - agenthub
source: system
read_only: true
---
# Claude Portability Manual

Use this manual when the task involves Claude Web, Claude exports, Claude memory, Claude projects, Claude skills, or restoring Agent Hub context back into Claude-compatible materials.

Treat Claude portability as a category-mapping task, not just a zip-upload task.
Claude's real data surface is broader than `memory + skills`.

This manual follows Claude's current public surfaces:

- `Profile preferences`
- `Styles`
- `Memory`
- `Standalone chats`
- `Projects`
- `Skills`
- `Connectors / external sources`
- `Export packages`

## What Claude Actually Has

### `Profile preferences`

- Account-wide response preferences.
- This is the closest Claude concept to durable Agent Hub profile data.

### `Styles`

- Claude styles change how Claude formats and presents responses.
- They are not the same thing as profile preferences or project instructions.
- Agent Hub does not have a first-class `style` entity, so preserve exact style text when it matters.

### `Memory`

- Claude has account-level memory and project-specific memory summaries.
- Claude also has a separate memory import/export flow.
- Do not mix `memory summary`, `manual memory edits`, and `chat history` into one bucket.

### `Standalone chats`

- Non-project chat history lives separately from projects.
- Incognito chats are a special case: they are not in normal chat history, and only show up in org-level export/compliance contexts.

### `Projects`

- A Claude project can include:
  - project instructions
  - project knowledge / uploaded files
  - project chats
  - a separate project memory summary
- Keep these subtypes separate when mapping into Agent Hub.

### `Skills`

- Skills are reusable cross-chat packages.
- They are not the same thing as project knowledge.
- One skill and many skills should not use the same import path.

### `Connectors / external sources`

- Connectors give Claude access to external apps and files.
- Usually this is setup metadata plus externally hosted content, not a portable Claude-owned file bundle.

### `Export packages`

- `Claude exported data zip`: the official account export from Claude settings.
- `Claude memory export`: the separate memory export/import flow.
- `Claude Web skills workspace zip`: a full zip of `/mnt/skills/user` from the Claude Web sandbox.

## Interface Layering Rules

- Use `update_profile` for durable account-wide preferences, principles, and stable working rules.
- Use `save_memory` for dated notes, extracted facts, scratch material, and small derived memories.
- Use `create_project`, `write_file`, `log_action`, and `get_project` to rebuild project structure manually.
- Use `import_skill` as the formal public MCP path for one skill whose files can be represented as a `map[path]string`. Nested relative paths are allowed, so text and code files such as `SKILL.md`, prompts, `.py`, `.js`, `.ts`, `.sh`, `.json`, `.yaml`, `.xml`, and `.xsd` can stay on this path.
- Use `import_skills_archive` as the formal public MCP path for multiple skills, binary-heavy skills, mixed text+asset skills, or any case where preserving the exact directory bundle is simpler than flattening files into a string map.
- Use `write_file` only to patch or repair one file after the main import path has already been chosen.
- Use `create_skills_import_token` plus `/agent/import/skills` as the user-mediated fallback transport path when a full skills zip cannot be transferred reliably through one MCP tool call. This is valid inside Claude Web too, especially when base64 or tool-call size limits block `import_skills_archive`.
- All skill import paths land under `/skills/<name>/...` in Agent Hub. Do not ask the user for a separate destination directory.
- Keep `Claude exported data zip` separate from skills archive flows. It currently uses `/api/import/claude-data` and does not have public MCP parity.
- Keep `Claude memory export` separate from full account export. It currently uses `/api/import/claude-memory` and does not have public MCP parity.

## Category Map

| Claude category | Claude source | Agent Hub target | Preferred interface | Fallback interface | Current parity / notes |
| --- | --- | --- | --- | --- | --- |
| Profile preferences | account-wide preference text | `/memory/profile/preferences` | `update_profile`, `read_profile` | `/api/import/profile` | Strong direct mapping |
| Styles | Claude style presets or custom styles | `/memory/profile/preferences`, archive note if exact style text matters | `update_profile` for stable rules, `write_file` for exact style archive | none | No first-class `style` object in Agent Hub |
| Claude memory summary or exported memory text | account memory, manual memory edits, memory export | `/memory/profile/*`, `/memory/scratch/*`, `/memory/claude/memory.md` | `/api/import/claude-memory` when exported memory text is available; otherwise `update_profile` + `save_memory` | `write_file` for archive notes | Public MCP parity does not exist for the Claude memory import HTTP path |
| Standalone chats | non-project chat history | `/memory/conversations/*.md` or archive notes | `/api/import/claude-data` when the official export zip is available | `write_file`, `save_memory` | No first-class public MCP conversation importer |
| Project instructions | project-level guidance | `/projects/<name>/context.md` | `create_project`, `write_file`, `get_project` | archive note under `/projects/<name>/...` | Manual reconstruction path |
| Project knowledge / uploaded files | project knowledge base, docs, code snippets, attached files | `/projects/<name>/...` when rebuilding manually; `/skills/claude-<project>/...` via current full export importer | `create_project`, `write_file`, `list_directory`, `read_file` | `/api/import/claude-data` | Current full export importer does not rebuild first-class Agent Hub projects |
| Project chats and project memory summary | chats inside a project, project-specific memory | archive notes, `/memory/conversations/*.md`, project notes | `/api/import/claude-data` for exported conversations; otherwise manual archive with `write_file` | `save_memory` for distilled facts | No first-class public MCP importer for project chats or project memory |
| Single text/code skill directory | one Claude skill whose files are all text-based and can be represented as strings, including nested paths like `scripts/run.py` | `/skills/<name>/...` | `import_skill(name, files)` | `import_skills_archive` | Good for `SKILL.md`, prompts, Python/source files, configs, and other text assets |
| Claude Web skills workspace zip | `/mnt/skills/user` full workspace zip, or any multi-skill / binary-heavy zip | `/skills/<name>/...` | `import_skills_archive` | `create_skills_import_token` + `/agent/import/skills` when one tool call cannot carry the archive reliably | Must preserve full directories, scripts, prompts, and assets |
| Connectors / external sources | connected services, selected repos/files, imported external context | `/projects/<name>/...`, setup notes, archive manifests | `write_file`, `log_action`, `search_memory` | manual recreation notes | Usually preserve setup metadata, not third-party data ownership |
| Official full data export zip | official Claude account export | `/memory/claude/memory.md`, `/memory/conversations/*.md`, `/skills/claude-<project>/...` | `/api/import/claude-data` | none on the public MCP surface | Current importer expects `users.json`, `memories.json`, `projects.json`, `conversations.json` |
| Account/user metadata from export | `users.json` inside the full export zip | archive note only if manually preserved | `write_file` if manually archiving extracted metadata | none | Current full export importer does not map `users.json` into a first-class Agent Hub domain |

## Import Checklist

1. Inventory the Claude-side categories first and mark each one `available`, `missing`, or `blocked`:
   `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`, `official exports`.
2. If the user already has the official `Claude exported data zip`, route that package to `/api/import/claude-data` first and explicitly note that there is no public MCP equivalent yet.
3. If the user has exported Claude memory text, prefer `/api/import/claude-memory`; otherwise split the content into durable profile rules for `update_profile` and smaller derived notes for `save_memory`.
4. Import `profile preferences` with `update_profile`. Do not bury stable account-wide rules inside scratch memory.
5. Import `styles` by extracting the stable formatting and communication rules into `update_profile`, and preserve the exact style text as an archive note when the exact wording matters.
6. Handle `standalone chats` separately from memory summaries. Use the full export path when available; otherwise preserve them as archive notes or files instead of pretending there is direct conversation parity.
7. Rebuild `project instructions` into `/projects/<name>/context.md` with `create_project` plus `write_file`.
8. Rebuild `project knowledge` into `/projects/<name>/...` when doing manual reconstruction. If using the official full export zip, note that the current importer writes project docs under `/skills/claude-<project>/...` rather than creating first-class Agent Hub projects.
9. Preserve `project chats` and `project memory summary` as archive notes or conversation files unless the full export importer is being used. Do not claim first-class public MCP parity here.
10. For `skills`, choose the path by payload shape instead of by platform name alone:
    - one small or moderate skill, all files are text/code and can be represented as `map[path]string` -> `import_skill(name, files)`
    - text/code files can still include nested paths like `scripts/run.py`, `prompts/review.txt`, `data/schema.xsd`, or `bin/tool.sh`
    - many skills, binary assets, or any archive where exact bytes matter -> `import_skills_archive`
    - Claude Web archive too large for one MCP tool call, or direct archive transfer is blocked by tool-call/base64 limits -> `create_skills_import_token` and let the user upload the full zip through `/agent/import/skills`
11. All skill import flows land under `/skills/<skill-name>/...` in Agent Hub. A fallback upload page or upload command should target the `/skills` root by default and should not ask the user to pick another destination directory.
12. Do not create or recommend markdown-only skills exports. Preserve `SKILL.md`, scripts, prompts, and binary assets together.
13. If a skills archive is too large, split it by top-level skill directories and import multiple full-skill batches. Do not split one skill directory into partial fragments unless a future chunked import flow exists.
14. If `create_skills_import_token` is chosen, package one complete zip first, then hand off the upload step to the user instead of trying to inline a truncated archive through MCP.
15. Preserve `connectors / external sources` as setup metadata, selected-file manifests, or project notes. Do not claim that third-party service data has been imported unless those files were actually captured.
16. End with a report that lists `imported`, `archived`, and `blocked` items, with the exact interface used for each category.

### Preferred Claude Web skills archive flow

When `/mnt/skills/user` exists and the goal is to move Claude Web skills into Agent Hub, prefer a full archive:

```bash
cd /mnt/skills/user
zip -r /mnt/user-data/outputs/agenthub-skills.zip .
```

Then choose the transport path that matches the payload:

1. If this is really one text/code skill and the files can be represented as strings, skip the zip and call `import_skill`. The `files` map may include nested paths like `scripts/main.py`.
2. Otherwise, prefer the MCP-native archive path:
   - read the zip bytes
   - base64-encode the full archive
   - call `import_skills_archive` with `platform="claude-web"` and the original archive name
3. If the archive is too large for one tool call, or Claude Web cannot pass the full base64 reliably, create a short-lived upload token and hand the full zip to the user for the `/agent/import/skills` fallback upload flow.

If an archive remains too large, split it into multiple zips by top-level skill directories and import multiple full-skill batches.

All of these paths still import into the Agent Hub `/skills` root.

### User-mediated browser upload fallback

When `create_skills_import_token` is used for Claude Web skill import, the agent should switch from "direct import" mode to "package + user handoff" mode.

Preferred flow:

1. Package one complete skills zip that preserves the original skill directories.
2. Make that zip available to the user as a downloadable file through the platform's file handoff/download mechanism.
3. Call `create_skills_import_token`.
4. If the token response includes a browser upload link, present that link to the user as the normal-user path.
5. Tell the user exactly what to do:
   - download the generated skills zip first
   - open the browser upload link returned by Agent Hub
   - upload the downloaded zip there
   - wait for the page to show success or failure
6. If the token response also includes a curl command, present it as the developer-friendly alternative, not the default path for ordinary users.
7. Make it explicit that the browser upload page imports into the Agent Hub `/skills` root by default. The user should not choose or type another destination path.

Suggested agent wording:

> I packaged your Claude Web skills as a zip. Please download that zip, open the Agent Hub upload link I generated, and upload the zip there. The page will import the contents into your Agent Hub `/skills` root and show which skills were imported or why the upload failed.

If the browser upload link is unavailable but a raw upload URL and token are available, the agent may still present the curl path to advanced users. Do not ask ordinary users to manually build multipart requests if a browser upload link exists.

## Export Checklist

1. Inventory Agent Hub data by Claude category, not just by file location:
   `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`.
2. Export durable profile rules from `read_profile` into Claude-ready `profile preferences`.
3. If the user wants a Claude `style`, derive it from the formatting and communication rules in profile or archived notes, and mark it as a manual Claude style setup step.
4. Export memory as either:
   - a concise Claude memory import text block, or
   - manual notes the user can paste into Claude's memory import flow.
5. Treat `/memory/conversations/*.md` as chat archive/reference material unless the user explicitly wants manual conversation restoration notes.
6. Rebuild Claude `project instructions` from `/projects/<name>/context.md`.
7. Rebuild Claude `project knowledge` from `/projects/<name>/...` and related files.
8. Rebuild `skills` one directory at a time for single-skill cases, or as a full archive for many skills or asset-heavy skills.
9. Recreate `connectors / external sources` as manual setup instructions and selected-file manifests. Do not claim that Agent Hub can restore third-party app connections natively.
10. Mark every manual restore step explicitly. Do not claim native Claude restore parity where it does not exist.
11. End with a report that lists generated materials, manual follow-ups, and remaining gaps.

## Current Gaps

- There is no public MCP equivalent for `/api/import/claude-memory`.
- There is no public MCP equivalent for `/api/import/claude-data`.
- The full Claude export importer currently lands memory under `/memory/claude/memory.md`, conversations under `/memory/conversations/*.md`, and project documents under `/skills/claude-<project>/...`; this is useful but not full first-class project parity.
- The current full Claude export importer does not map `users.json` into a first-class Agent Hub domain.
- There is no first-class public MCP importer for standalone chats, project chats, or project memory summaries.
- `import_skill` is the right path for one text/code skill whose files can be represented as strings, including nested paths such as `scripts/*.py`. Use `import_skills_archive` when the skill is large, multi-skill, or includes binary assets.
- `create_skills_import_token` plus `/agent/import/skills` is the user-mediated fallback transport path when one MCP tool call cannot carry the full archive reliably. This can be the right fallback inside Claude Web.
- If one archive is too large, split it by whole skill directories and import multiple batches. True chunked import for one oversized skill does not exist yet.

## Prompt Template

Use or adapt this prompt when another agent needs to execute Claude portability work:

> Read `/skills/portability/claude/SKILL.md` first. Inventory the Claude-side categories as `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`, and `official exports`. Map each category to the nearest Agent Hub domain instead of mixing them together. Use `update_profile` for durable account-wide rules, `save_memory` for smaller derived notes, `create_project` plus `write_file` for project reconstruction, `import_skill` only for one text/code skill whose files can be represented as strings, and `import_skills_archive` for `/mnt/skills/user` or other multi-skill / binary-heavy archives. If one archive is too large for a single MCP tool call, switch to `create_skills_import_token` and the user-mediated `/agent/import/skills` upload flow instead of truncating base64. In that fallback flow, package one complete zip, hand the zip to the user for download, then tell the user to open the returned browser upload link and upload the zip manually. All skill imports land under `/skills/<name>/...` in Agent Hub, and the browser upload page targets the `/skills` root by default. If the user already has the official full Claude export zip, note that `/api/import/claude-data` is the preferred path and that there is no public MCP parity yet. Preserve unsupported structures as archive notes instead of dropping them, and finish with `imported`, `archived`, and `blocked` items plus the exact interface used for each category.

{{CURRENT_USER_SNAPSHOT}}
