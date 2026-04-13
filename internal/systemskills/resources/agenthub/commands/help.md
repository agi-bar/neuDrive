# `agenthub help`

Use this command when the user asks what Agent Hub can do from the current platform.

## Public Forms

- `agenthub help`
- `agenthub help roots`
- `agenthub help <command>`
- `agenthub help project`

## Explain

- Agent Hub now uses a root-directory mental model:
  - `agenthub ls [path]`
  - `agenthub read <path>`
  - `agenthub write <path> <content-or-file>`
  - `agenthub search <query> [path]`
  - `agenthub create project <name>`
  - `agenthub log project/<name> ...`
  - `agenthub import <category> <src>`
  - `agenthub token create --kind sync|skills-upload`
  - `agenthub stats`
- The external roots are `profile`, `memory`, `project`, `skill`, `secret`, and `platform`.
- A leading `/` is optional. `project/demo` and `/project/demo` are equivalent.
- `agenthub platform ...`, `connect`, `disconnect`, `export`, `git`, and `status` remain operational commands.
- `agenthub git init [--output DIR]` prepares a local Git repo mirror of the current local Hub data, excluding secrets.
- `agenthub git pull` refreshes that local mirror on demand.
- Once the user has initialized that local mirror, later Hub writes and imports keep syncing into the same directory automatically.

## Good Guidance

- Start with `agenthub help` when the user needs the whole mental model.
- Use `agenthub help roots` when the user is confused about `profile / memory / project / skill / secret / platform`.
- `agenthub help project`, `agenthub help memory`, and similar root names should resolve back to the path model guidance.
- Use `agenthub help write`, `agenthub help import`, or `agenthub help git` when the user needs one concrete workflow with examples.
- For Claude/Codex embedded usage, mirror the same guidance with `/agenthub help ...` or `$agenthub help ...`.
