# `agenthub write`

Use this command when the user wants to create or update Agent Hub data from text or a local file path.

## Examples

- `agenthub write profile/preferences ./preferences.md`
- `agenthub write memory "Remember this"`
- `agenthub write project/demo/notes.md ./notes.md`
- `agenthub write skill/writer/SKILL.md ./SKILL.md`

## Notes

- The second argument may be literal text, `-` for stdin, or a local file path.
- Use `--literal` when an argument that looks like a local path should be treated as plain text.
- `secret` is read-only in the current public surface.
