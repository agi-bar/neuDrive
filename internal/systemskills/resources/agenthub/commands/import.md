# `agenthub import`

Use this command when the user wants to restore Agent Hub data back into the current platform.

## Steps

1. Read `/skills/agenthub/SKILL.md`.
2. Read the platform reference under `/skills/agenthub/references/platforms/<platform>.md`.
3. Prefer exact Agent Hub assets when they exist.
4. Recreate platform-native materials from Agent Hub profile, projects, skills, and metadata.
5. Mark every manual restoration step explicitly.
6. If the user already initialized a local Git mirror with `agenthub git init`, remind them that later Hub writes and imports are mirrored there automatically, but GitHub push still requires manual Git commands in that directory.

## Guardrails

- Do not claim full parity when the platform cannot natively represent an Agent Hub concept.
- Keep exact file exports separate from derived setup notes.
- Preserve unsupported mappings as archive or notes.
- Do not imply that secrets were exported into the local Git mirror; only non-secret metadata is mirrored.
