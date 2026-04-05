# ChatGPT Portability Examples

## Import Prompt

Use this prompt when the user wants to back up ChatGPT data into AgentHub:

> Read `/skills/portability/chatgpt/SKILL.md` first. Then classify the user's ChatGPT data into profile, memory, projects, knowledge/files, tools/connections, automations, and conversations. Write stable data into native AgentHub domains and preserve everything else as structured archive instead of dropping it.

## Export Prompt

Use this prompt when the user wants to restore AgentHub data into ChatGPT:

> Read `/skills/portability/chatgpt/SKILL.md` first. Generate ChatGPT-compatible Custom Instructions text, project seed documents, a knowledge upload manifest, and draft GPT Actions configuration. Mark every manual follow-up step explicitly.

## Reporting Template

End with:

- Summary
- Native mappings completed
- Archived or shadow-preserved items
- Manual follow-ups
- Unsupported or unknown items
