# `neudrive import`

Use this command when the user wants to bring local or platform data into neuDrive.

## Public Forms

- `neudrive import platform <platform> [--mode agent|files|all] [--zip FILE]`
- `neudrive import skill <local-dir> [--name NAME]`
- `neudrive import profile <local-file> [--category preferences|relationships|principles]`
- `neudrive import memory <local-file-or-dir>`
- `neudrive import project <local-file-or-dir> [--name NAME]`

## Notes

- Categories come after the verb.
- A leading `/` is optional when the user writes category-like paths.
- If the user already enabled local Git Mirror, remind them that later Hub writes and imports are mirrored there automatically, but GitHub push still requires normal Git credentials and repo setup in that directory.
