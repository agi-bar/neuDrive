# `agenthub git`

Use this command when the user wants to mirror the current local Agent Hub into a local Git repository or refresh that mirror.

## Subcommands

- `agenthub git init [--output DIR]`
  Export all non-secret local Hub data into a local folder, run `git init` when needed, and register that folder as the active mirror for future syncs.
- `agenthub git pull`
  Refresh the active local Git mirror from the current local Hub state.

## Steps

1. Read `/skills/agenthub/SKILL.md`.
2. This flow is local-only and works in Agent Hub local mode regardless of whether the local backend is SQLite or Postgres.
3. For a first-time repo mirror, use `agenthub git init [--output DIR]`.
4. For a later manual refresh, use `agenthub git pull`.
5. Tell the user that secrets are not exported; vault only exposes scope metadata in the mirror.
6. Report the local mirror path explicitly.
7. Remind the user that GitHub sync still requires them to run `git add / git commit / git remote add origin / git push` in that directory.
8. If no mirror has been initialized yet, tell the user to run `agenthub git init` first instead of pretending that `git pull` can create one.
