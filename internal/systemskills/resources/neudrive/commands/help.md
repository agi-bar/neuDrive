# `neudrive help`

Use this command when the user asks what neuDrive can do from the current platform.

## Public Forms

- `neudrive help`
- `neudrive help roots`
- `neudrive help <command>`
- `neudrive help project`

## Explain

- neuDrive now uses a root-directory mental model:
  - `neudrive ls [path]`
  - `neudrive read <path>`
  - `neudrive write <path> <content-or-file>`
  - `neudrive search <query> [path]`
  - `neudrive create project <name>`
  - `neudrive log project/<name> ...`
  - `neudrive import <category> <src>`
  - `neudrive token create --kind sync|skills-upload`
  - `neudrive stats`
- The external roots are `profile`, `memory`, `project`, `skill`, `secret`, and `platform`.
- A leading `/` is optional. `project/demo` and `/project/demo` are equivalent.
- `neudrive platform ...`, `connect`, `disconnect`, `export`, `git`, and `status` remain operational commands.
- `neudrive git init [--output DIR]` prepares a local Git repo mirror of the current local Hub data, excluding secrets.
- `neudrive git pull` refreshes that local mirror on demand.
- Once the user has initialized that local mirror, later Hub writes and imports keep syncing into the same directory automatically.

## Good Guidance

- Start with `neudrive help` when the user needs the whole mental model.
- Use `neudrive help roots` when the user is confused about `profile / memory / project / skill / secret / platform`.
- `neudrive help project`, `neudrive help memory`, and similar root names should resolve back to the path model guidance.
- Use `neudrive help write`, `neudrive help import`, or `neudrive help git` when the user needs one concrete workflow with examples.
- For Claude/Codex embedded usage, mirror the same guidance with `/neudrive help ...` or `$neudrive help ...`.
