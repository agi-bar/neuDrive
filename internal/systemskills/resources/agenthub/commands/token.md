# `agenthub token`

Use this command when the user needs a short-lived token for sync or prepared skills upload workflows.

## Public Form

- `agenthub token create --kind sync --purpose backup [--access push|pull|both]`
- `agenthub token create --kind skills-upload --purpose skills [--platform claude-web]`

## Notes

- `sync` replaces the old `create_sync_token` mental model.
- `skills-upload` replaces the old `prepare_skills_upload` mental model.
