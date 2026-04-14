# `neudrive git`

Use this command when the user wants to mirror the current local neuDrive into a local Git repository or refresh that mirror.

## Subcommands

- `neudrive git init [--output DIR]`
  Export all non-secret local Hub data into a local folder, run `git init` when needed, and register that folder as the active mirror for future syncs. If `--output` is omitted, use `local.git_mirror_path` from the local `config.json`; if it is missing, write the default `./neudrive-export/git-mirror` into `config.json` first.
- `neudrive git pull`
  Refresh the active local Git mirror from the current local Hub state.

## Steps

1. Read `/skills/neudrive/SKILL.md`.
2. This flow is local-only and works in neuDrive local mode regardless of whether the local backend is SQLite or Postgres.
3. For a first-time repo mirror, use `neudrive git init [--output DIR]`.
4. For a later manual refresh, use `neudrive git pull`.
5. Tell the user that secrets are not exported; vault only exposes scope metadata in the mirror.
6. Report the local mirror path explicitly.
7. Remind the user that GitHub sync still requires them to run `git add / git commit / git remote add origin / git push` in that directory.
8. If no mirror has been initialized yet, tell the user to run `neudrive git init` first instead of pretending that `git pull` can create one.
