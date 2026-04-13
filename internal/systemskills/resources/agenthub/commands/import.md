# `agenthub import`

Use this command when the user wants to bring local or platform data into Agent Hub.

## Public Forms

- `agenthub import platform <platform> [--mode agent|files|all] [--zip FILE]`
- `agenthub import skill <local-dir> [--name NAME]`
- `agenthub import profile <local-file> [--category preferences|relationships|principles]`
- `agenthub import memory <local-file-or-dir>`
- `agenthub import project <local-file-or-dir> [--name NAME]`

## Notes

- Categories come after the verb.
- A leading `/` is optional when the user writes category-like paths.
- If the user already initialized a local Git mirror with `agenthub git init`, remind them that later Hub writes and imports are mirrored there automatically, but GitHub push still requires manual Git commands in that directory.
