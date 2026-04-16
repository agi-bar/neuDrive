English | [简体中文](cli.zh-CN.md)

# neuDrive CLI Guide

This is the detailed CLI guide linked from the README. For platform-by-platform connection setup, see the [Setup Guide](setup.md).

## Install

```bash
./tools/install-neudrive.sh
```

Or:

```bash
make install
```

## Quick Start

```bash
neudrive status
neudrive platform ls
neudrive connect claude
neudrive browse
```

## Built-In Help

```bash
neudrive help
neudrive help roots
neudrive help write
```

## Core Hub Commands

These commands work against the public neuDrive roots such as `profile`, `memory`, `project`, `skill`, `secret`, and `platform`.

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive ls [path]` | Browse the public roots or a subtree | `neudrive ls project/demo` |
| `neudrive read <path>` | Read one Hub path as text, summary data, or a secret value | `neudrive read profile/preferences` |
| `neudrive write <path> <content-or-file>` | Create or update Hub content from text, stdin, or a local file | `neudrive write project/demo/docs/notes.md ./notes.md` |
| `neudrive search <query> [path]` | Search Hub content globally or under one path scope | `neudrive search migration project/demo` |
| `neudrive create project <name>` | Create a project | `neudrive create project launch-plan` |
| `neudrive log <project-path> --action ... --summary ...` | Append a structured log entry to a project | `neudrive log project/demo --action note --summary "Kickoff complete"` |
| `neudrive stats` | Show a quick content summary for the current Hub | `neudrive stats` |

## Local Runtime Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive status` | Check whether the local daemon and storage are ready | `neudrive status` |
| `neudrive browse [--print-url] [/route]` | Open the local dashboard or print its authenticated URL | `neudrive browse /data/files` |
| `neudrive doctor` | Run a concise readiness diagnostic | `neudrive doctor` |
| `neudrive daemon status` | Show daemon status | `neudrive daemon status` |
| `neudrive daemon logs [--tail N]` | Show recent daemon logs | `neudrive daemon logs --tail 50` |
| `neudrive daemon stop` | Stop the local daemon | `neudrive daemon stop` |

## Platform Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive platform ls` | List installed adapters and connection state | `neudrive platform ls` |
| `neudrive platform show <platform>` | Show paths, entrypoints, and usage hints for one adapter | `neudrive platform show claude` |
| `neudrive connect <platform>` | Install or refresh the managed neuDrive entrypoint for a platform | `neudrive connect claude` |
| `neudrive disconnect <platform>` | Remove a managed entrypoint and its local metadata | `neudrive disconnect claude` |
| `neudrive export <platform> [--output DIR]` | Stage platform-shaped export materials from the current local Hub | `neudrive export claude --output ./claude-export` |

## Import Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive import platform <platform> [--mode ...] [--zip FILE]` | Import platform data such as Codex or Claude captures | `neudrive import platform codex` |
| `neudrive import skill <dir> [--name NAME]` | Import one local skill directory | `neudrive import skill ./demo-skill` |
| `neudrive import profile <file> [--category ...]` | Import one profile document | `neudrive import profile ./preferences.md --category preferences` |
| `neudrive import memory <file-or-dir>` | Import scratch or note-style memory content | `neudrive import memory ./notes` |
| `neudrive import project <file-or-dir> [--name NAME]` | Import project files into a neuDrive project | `neudrive import project ./demo-project --name demo` |

## Git Mirror Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive git init [--output DIR]` | Export non-secret local Hub data into a Git mirror and register it | `neudrive git init --output ./neudrive-export/git-mirror` |
| `neudrive git pull` | Refresh the active Git mirror from the current local Hub state | `neudrive git pull` |
| `neudrive git auth github-app --device` | Connect your GitHub App user account for Git mirror workflows | `neudrive git auth github-app --device` |

## Token Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive token create --kind sync ...` | Create a short-lived sync token | `neudrive token create --kind sync --purpose backup --access both` |
| `neudrive token create --kind skills-upload ...` | Create a short-lived skills-upload token | `neudrive token create --kind skills-upload --purpose skills --platform claude-web` |

## Hosted Cloud And Remote Profiles

Use these commands when you want named remote profiles outside the bundle-oriented sync surface.

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive remote login <profile> [--url URL] [--token TOKEN]` | Log in to a named remote profile; `official` defaults to `https://neudrive.ai` | `neudrive remote login official` |
| `neudrive remote ls` | List saved remote profiles | `neudrive remote ls` |
| `neudrive remote use <profile>` | Switch the current profile | `neudrive remote use official` |
| `neudrive remote whoami <profile>` | Show current auth state for one profile | `neudrive remote whoami official` |
| `neudrive remote logout [profile]` | Clear the saved token for a profile | `neudrive remote logout official` |

## Bundle Sync Commands

Use `sync` when you want archive-style import/export flows against a remote neuDrive profile.

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive sync login --profile NAME` | Browser login and save a sync profile | `neudrive sync login --profile official` |
| `neudrive sync profiles` | List configured sync profiles | `neudrive sync profiles` |
| `neudrive sync use [--profile NAME \| NAME]` | Switch the active sync profile | `neudrive sync use official` |
| `neudrive sync whoami [--profile NAME]` | Show identity and scopes for the current sync profile | `neudrive sync whoami --profile official` |
| `neudrive sync logout --profile NAME` | Clear the saved sync token for one profile | `neudrive sync logout --profile official` |
| `neudrive sync export --source DIR [--format json\|archive] [--output FILE]` | Build an export bundle from a local source directory | `neudrive sync export --source ./skills --output backup.ndrv` |
| `neudrive sync preview --source DIR \| --bundle FILE` | Preview an incoming bundle without applying it | `neudrive sync preview --bundle backup.ndrv` |
| `neudrive sync push --source DIR \| --bundle FILE` | Push a source directory or bundle into a remote Hub | `neudrive sync push --bundle backup.ndrv` |
| `neudrive sync pull [--format json\|archive] [--output FILE]` | Pull content from a remote Hub into a local bundle file | `neudrive sync pull --format archive --output pulled.ndrvz` |
| `neudrive sync resume --bundle FILE [--session-file FILE]` | Resume an interrupted archive upload session | `neudrive sync resume --bundle backup.ndrvz` |
| `neudrive sync history` | Show recent sync sessions | `neudrive sync history` |
| `neudrive sync diff --left FILE --right FILE [--format text\|json]` | Compare two bundles and return non-zero when they differ | `neudrive sync diff --left before.ndrv --right after.ndrv` |

## Low-Level Server Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neudrive server [flags]` | Run the standalone neuDrive HTTP server | `neudrive server --listen 127.0.0.1:42690 --local-mode` |
| `neudrive mcp stdio [flags]` | Run the neuDrive MCP server over stdio | `neudrive mcp stdio --token-env NEUDRIVE_TOKEN` |

## Help

Use the built-in help when you need command-specific syntax:

```bash
neudrive help
neudrive help roots
neudrive help write
```

For testing coverage rather than day-to-day usage, see the [CLI test matrix](cli-test-matrix.md).
