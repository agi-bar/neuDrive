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

For Claude skills work, read this manual before choosing `import_skill`, `import_skills_archive`, or `prepare_skills_upload`.
If the user says "all skills", `/mnt/skills/user`, `public + examples`, workspace zip, or any other multi-skill request, do not start with `import_skill` and do not reduce the migration to `SKILL.md` only.

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
- Use `create_project`, `log_action`, and `get_project` to rebuild project structure manually when the imported data really belongs to a Claude project.
- Use `write_file` for any imported file-like data that should be preserved in Agent Hub, not just for projects. If some Claude material does not fit profile, memory, project, or skill cleanly, it may still be imported under a sensible custom directory structure.
- Use `import_skill` as the formal public MCP path for one skill whose files can be represented as a `map[path]string`. Nested relative paths are allowed, so text and code files such as `SKILL.md`, prompts, `.py`, `.js`, `.ts`, `.sh`, `.json`, `.yaml`, `.xml`, and `.xsd` can stay on this path.
- Even on the `import_skill` path, import the whole skill directory, not just `SKILL.md`. Include every text/code file the skill needs, such as `scripts/`, `prompts/`, config files, schemas, and helper sources.
- If the user asks for all Claude skills, a full skills workspace, or any multi-skill bundle, do not use `import_skill` as the primary transport. Package the full directories first and choose `prepare_skills_upload` or, only when already known small enough, `import_skills_archive`.
- Use `import_skills_archive` only for archives already known to be small enough for one MCP tool call. For Claude Web, that means the zip must already be known to be `<= 64 KB`. Do not `cat` base64(zip), paste archive base64 into chat, or otherwise emit long archive strings into the conversation, because that can crash the conversation session.
- The agent may use `write_file` to patch one file, archive unsupported exports, or preserve additional imported Claude data under custom paths when no first-class Agent Hub domain exists.
- For Claude Web skills zips, check the zip size before reading contents. If the zip is larger than `64 KB`, or if the size cannot be checked safely, do not read or base64 it into MCP args. Do not `cat` base64(zip) or otherwise print long archive strings into the conversation.
- Use `prepare_skills_upload` plus `/agent/import/skills` as the prepared upload path for Claude Web zips larger than `64 KB`, zips whose size cannot be checked safely, or any archive that should not be inlined into one tool call.
- After `prepare_skills_upload`, first POST the returned `connectivity_probe_url`. If it returns `200`, upload directly from the Claude sandbox with the returned `curl_example` and do not hand the zip off to the user.
- If the probe POST fails in Claude Web, treat it as an outbound-domain restriction. Tell the user to open `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains` and add the Agent Hub host, such as `agenthub.agi.bar` on the official service. Then retry the probe and direct upload.
- If the user has already added the domain and the probe still fails in the current Claude Web conversation, explain that Claude Web may require a new conversation before the new allowed-domain setting takes effect. Ask whether they want to start a new conversation and retry, or use the fallback now.
- If the user does not add the domain, does not want to start a new conversation, or direct upload is still blocked, fall back to the returned `browser_upload_url` or ask the user to run the returned `curl_example` manually.
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
| Single text/code skill directory | one Claude skill whose files are all text-based and can be represented as strings, including nested paths like `scripts/run.py` | `/skills/<name>/...` | `import_skill(name, files)` | `import_skills_archive` for a small exact-byte archive, or `prepare_skills_upload` when the archive should not be inlined | Good for `SKILL.md`, prompts, Python/source files, configs, and other text assets. Still send the whole skill directory, not just `SKILL.md`. |
| Claude Web skills workspace zip | `/mnt/skills/user` full workspace zip, or any multi-skill / binary-heavy zip | `/skills/<name>/...` | `prepare_skills_upload`, then POST `connectivity_probe_url`, then direct `curl_example` upload when the probe returns `200` | `browser_upload_url`, user-run `curl_example`, or `import_skills_archive` only when the zip is already known to be `<= 64 KB` | Must preserve full directories, scripts, prompts, and assets. Do not read or base64 a larger Claude Web zip into MCP args. |
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
    - do not simplify a skill to only `SKILL.md`; if `scripts/`, prompts, config, schemas, or helper files exist, include them too
    - if the user asks for all skills, `/mnt/skills/user`, `public + examples`, a workspace export, or any other multi-skill batch -> do not use `import_skill`; package one full archive first
    - for Claude Web zips, check the zip size before reading contents; if the zip is larger than `64 KB` or the size cannot be checked safely -> `prepare_skills_upload`
    - many skills, binary assets, or any archive where exact bytes matter -> `import_skills_archive` only when the archive is already known small enough for one MCP tool call
11. All skill import flows land under `/skills/<skill-name>/...` in Agent Hub. A fallback upload page or upload command should target the `/skills` root by default and should not ask the user to pick another destination directory.
12. Do not create or recommend markdown-only or `SKILL.md`-only skill exports. Preserve the entire skill directory and keep `SKILL.md`, scripts, prompts, config, schemas, and binary assets together.
13. If the user still wants pure inline MCP transport, split a Claude Web archive by top-level skill directories only when each resulting zip is known to be `<= 64 KB`. Do not split one skill directory into partial fragments unless a future chunked import flow exists.
14. If `prepare_skills_upload` is chosen, package one complete zip first, then try the direct sandbox upload path before involving the user.
15. First POST the returned `connectivity_probe_url`.
16. If that POST returns `200`, use the returned `curl_example` from the Claude sandbox and finish the upload directly.
17. If the probe POST fails in Claude Web, tell the user to add the Agent Hub host to `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains`, then retry the probe and direct upload.
18. If the user has already added the domain and the probe still fails in the current Claude Web conversation, tell the user that the new setting may only take effect in a new conversation. Ask whether they want to start a new conversation and retry, or use the fallback now.
19. If the user does not add the domain, does not want to start a new conversation, or direct upload is still blocked, use `browser_upload_url` for ordinary users or ask the user to run the returned `curl_example` manually.
19. When a Claude export contains useful files or metadata that do not fit a first-class Agent Hub domain, preserve them with `write_file` under a clear custom directory structure instead of forcing them into a project.
20. Preserve `connectors / external sources` as setup metadata, selected-file manifests, or project notes. Do not claim that third-party service data has been imported unless those files were actually captured.
21. End with a report that lists `imported`, `archived`, and `blocked` items, with the exact interface used for each category.

### Preferred Claude Web skills archive flow

When `/mnt/skills/user` exists and the goal is to move Claude Web skills into Agent Hub, prefer a full archive:

```bash
cd /mnt/skills/user
zip -r /mnt/user-data/outputs/agenthub-skills.zip .
```

Then choose the transport path that matches the payload:

1. If this is really one text/code skill and the files can be represented as strings, skip the zip and call `import_skill`. The `files` map may include nested paths like `scripts/main.py`.
   - include the full skill directory contents, not just `SKILL.md`
   - do not use this branch when the user asked for all skills, `/mnt/skills/user`, `public + examples`, or any multi-skill workspace
2. Otherwise, stat the zip before reading contents.
   - if the Claude Web zip is larger than `64 KB`, do not read the bytes
   - if the Claude Web zip size cannot be checked safely, do not read the bytes
3. If the Claude Web zip is larger than `64 KB`, or the size is unknown, call `prepare_skills_upload`.
4. First POST the returned `connectivity_probe_url`.
   - if the probe returns `200`, use the returned `curl_example` from the Claude sandbox and finish the upload directly
   - if the probe fails in Claude Web, tell the user to add the Agent Hub host to `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains`, then retry
   - if the user has already added the domain and the probe still fails in the current Claude Web conversation, explain that they may need to start a new conversation for the setting to take effect
   - if the user does not add the domain, does not want to start a new conversation, or direct upload is still blocked, fall back to the returned `browser_upload_url` or ask the user to run the returned `curl_example` manually
5. Only if the Claude Web zip is already known to be `<= 64 KB`, read the zip bytes, base64-encode the archive, and call `import_skills_archive` with `platform="claude-web"` and the original archive name.
6. If the user insists on pure inline MCP transport, split the archive by top-level skill directories only when each resulting zip is known to stay within the same `64 KB` limit.

All of these paths still import into the Agent Hub `/skills` root.

### Prepared upload flow

When `prepare_skills_upload` is used for Claude Web skill import, the agent should switch from "inline MCP import" mode to "prepared upload" mode.

Preferred flow:

1. Package one complete skills zip that preserves the original skill directories.
   - do not omit `scripts/`, prompts, config files, schemas, fonts, or other helper assets
   - do not replace a full skill with a `SKILL.md`-only shortcut
2. Call `prepare_skills_upload`.
3. First POST the returned `connectivity_probe_url`.
4. If the probe returns `200`, upload the zip directly from the Claude sandbox with the returned `curl_example`.
   - this is the preferred path when Claude can reach the Agent Hub host directly
   - no user download or browser handoff is needed in this case
5. If the probe fails in Claude Web, explain that Claude likely blocked outbound POST to the Agent Hub host.
6. Tell the user exactly how to unblock direct upload:
   - open `Settings -> Capabilities -> Code execution and file creation`
   - add the Agent Hub host, such as `agenthub.agi.bar`, under `Additional allowed domains`
   - then retry the same probe and direct upload flow
7. If the user already added the domain and the same Claude Web conversation still cannot POST:
   - explain that the new allowed-domain setting may only take effect in a new conversation
   - ask whether they want to start a new conversation and retry, or use the fallback now
8. If the user does not add the domain, does not want to start a new conversation, or direct upload is still blocked:
   - use the returned browser upload link for ordinary users
   - or ask terminal-comfortable users to run the returned `curl_example` manually
9. When using the browser fallback, first make the generated zip available to the user as a downloadable file through the platform's file handoff/download mechanism.
10. Make it explicit that the browser upload page imports into the Agent Hub `/skills` root by default. The user should not choose or type another destination path.

Suggested agent wording:

> I packaged your Claude Web skills as a full zip, including all files under each skill directory, not just `SKILL.md`. I first tested whether Claude can POST directly to the Agent Hub host. If that probe succeeds, I will upload the zip directly from the Claude sandbox with the prepared curl command. If Claude blocks that POST, please open `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains`, add the Agent Hub host, and then I can retry the direct upload. If you already added it and this same Claude Web conversation still cannot POST, the new setting may require a new conversation before it takes effect. In that case, you can start a new conversation and retry, or I can fall back to the browser upload page or give you the curl command to run yourself.

If the token response includes both a browser upload link and a curl example, keep the direct sandbox curl path as the first choice after a successful probe. If direct upload stays blocked, use the browser path for ordinary users and the curl path for terminal-comfortable users. Do not ask ordinary users to manually build multipart requests if a browser upload link exists.

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
- `import_skill` is the right path for one text/code skill whose files can be represented as strings, including nested paths such as `scripts/*.py`. Even then, import the whole skill directory, not just `SKILL.md`.
- For Claude Web, do not inline a skills zip into `import_skills_archive` unless it is already known to be `<= 64 KB`. If the size is unknown, default to `prepare_skills_upload`.
- `prepare_skills_upload` plus `/agent/import/skills` is the prepared upload path for Claude Web archives that should not be inlined into one tool call.
- When `prepare_skills_upload` is used, direct sandbox upload via the returned `curl_example` is preferred after the returned `connectivity_probe_url` succeeds.
- If the probe fails in Claude Web, the user may need to add the Agent Hub host to `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains` before retrying direct upload.
- If the user already added the domain and the current Claude Web conversation still fails the probe, the new setting may require a new conversation before retrying direct upload.
- If the user still wants pure inline MCP transport, split by whole skill directories only when each resulting archive is known to stay within the same `64 KB` limit. True chunked import for one oversized skill does not exist yet.

## Prompt Template

Use or adapt this prompt when another agent needs to execute Claude portability work:

> Read `/skills/portability/claude/SKILL.md` first. Inventory the Claude-side categories as `profile preferences`, `styles`, `memory`, `standalone chats`, `project instructions`, `project knowledge`, `project chats`, `skills`, `connectors`, and `official exports`. Map each category to the nearest Agent Hub domain instead of mixing them together. Use `update_profile` for durable account-wide rules, `save_memory` for smaller derived notes, `create_project` for true Claude project reconstruction, and `write_file` for any additional imported Claude files or metadata that should be preserved even when they do not fit a first-class Agent Hub domain. The agent may design a sensible custom directory structure for those files. Use `import_skill` only for one text/code skill whose files can be represented as strings. If the user asks for all skills, `/mnt/skills/user`, `public + examples`, or any multi-skill workspace, do not use `import_skill`; package the full directories first. Never simplify a skill to `SKILL.md` only; include the whole skill directory and all needed `scripts/`, prompts, config, schemas, and assets. For Claude Web skills zips, check zip size before reading contents. If the zip is larger than `64 KB`, or if the size cannot be checked safely, do not read or base64 it into MCP args; switch to `prepare_skills_upload` instead. After `prepare_skills_upload`, first POST the returned `connectivity_probe_url`. If it returns `200`, use the returned `curl_example` to upload directly from the Claude sandbox. If that POST fails in Claude Web, tell the user to add the Agent Hub host to `Settings -> Capabilities -> Code execution and file creation -> Additional allowed domains`, then retry. If the user already added the domain and the probe still fails in the current Claude Web conversation, explain that the new setting may require a new conversation before it takes effect, and ask whether they want to start a new conversation and retry or use the fallback now. If the user does not add it, does not want to start a new conversation, or direct upload is still blocked, fall back to the returned browser upload link or ask the user to run the returned curl command manually. Only use `import_skills_archive` when the zip is already known to be `<= 64 KB`. All skill imports land under `/skills/<name>/...` in Agent Hub, and the browser upload page targets the `/skills` root by default. Preserve unsupported structures as archive notes, structured metadata, or custom file trees instead of dropping them, and finish with `imported`, `archived`, and `blocked` items plus the exact interface used for each category.

{{CURRENT_USER_SNAPSHOT}}
