# Claude Portability Examples

## Import Prompt

> Read `/skills/portability/claude/SKILL.md` first. If a Claude exported data zip exists, prioritize it. If the task is specifically to export Claude Web skills, package `/mnt/skills/user` into a single zip, call `create_skills_import_token`, and upload that zip into AgentHub so it expands into `/skills/<name>/...`. Then migrate Claude memory, projects, skills, and conversations into AgentHub. Preserve anything that does not map cleanly as archive.

## Export Prompt

> Read `/skills/portability/claude/SKILL.md` first. Generate Claude-compatible working instructions, project context prompts, and skill directory materials from AgentHub data. Mark all manual restore steps explicitly.

## Reporting Template

End with:

- Summary
- Native imports completed
- Archived items
- Manual follow-ups
- Unsupported or unknown parity
