English | [简体中文](cli.zh-CN.md)

# neuDrive CLI Guide

This is the detailed CLI guide linked from the README. For platform-by-platform connection setup, see the [Setup Guide](setup.md).

Examples below use `neu`; the `neudrive` alias remains supported.

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
neu status
neu platform ls
neu connect claude
neu browse
```

## Built-In Help

```bash
neu help
neu help roots
neu help write
```

## Core Hub Commands

These commands work against the public neuDrive roots such as `profile`, `memory`, `project`, `skill`, `secret`, and `platform`.

| Command | What it does | Example |
|---------|---------------|---------|
| `neu ls [path]` | Browse the public roots or a subtree | `neu ls project/demo` |
| `neu read <path>` | Read one Hub path as text, summary data, or a secret value | `neu read profile/preferences` |
| `neu write <path> <content-or-file>` | Create or update Hub content from text, stdin, or a local file | `neu write project/demo/docs/notes.md ./notes.md` |
| `neu search <query> [path]` | Search Hub content globally or under one path scope | `neu search migration project/demo` |
| `neu create project <name>` | Create a project | `neu create project launch-plan` |
| `neu log <project-path> --action ... --summary ...` | Append a structured log entry to a project | `neu log project/demo --action note --summary "Kickoff complete"` |
| `neu stats` | Show a quick content summary for the current Hub | `neu stats` |

## Local Runtime Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neu status` | Check whether the local daemon and storage are ready | `neu status` |
| `neu browse [--print-url] [/route]` | Open the local dashboard or print its authenticated URL | `neu browse /data/files` |
| `neu doctor` | Run a concise readiness diagnostic | `neu doctor` |
| `neu daemon status` | Show daemon status | `neu daemon status` |
| `neu daemon logs [--tail N]` | Show recent daemon logs | `neu daemon logs --tail 50` |
| `neu daemon stop` | Stop the local daemon | `neu daemon stop` |

## Platform Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neu platform ls` | List installed adapters and connection state | `neu platform ls` |
| `neu platform show <platform>` | Show paths, entrypoints, and usage hints for one adapter | `neu platform show claude` |
| `neu connect <platform>` | Install or refresh the managed neuDrive entrypoint for a platform | `neu connect claude` |
| `neu disconnect <platform>` | Remove a managed entrypoint and its local metadata | `neu disconnect claude` |
| `neu export <platform> [--output DIR]` | Stage platform-shaped export materials from the current local Hub | `neu export claude --output ./claude-export` |

## Import Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neu import platform <platform> [--mode ...] [--zip FILE]` | Import platform data such as Codex or Claude captures | `neu import platform codex` |
| `neu import skill <dir> [--name NAME]` | Import one local skill directory | `neu import skill ./demo-skill` |
| `neu import profile <file> [--category ...]` | Import one profile document | `neu import profile ./preferences.md --category preferences` |
| `neu import memory <file-or-dir>` | Import scratch or note-style memory content | `neu import memory ./notes` |
| `neu import project <file-or-dir> [--name NAME]` | Import project files into a neuDrive project | `neu import project ./demo-project --name demo` |

## Git Mirror Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neu git init [--output DIR]` | Export non-secret local Hub data into a Git mirror and register it | `neu git init --output ./neudrive-export/git-mirror` |
| `neu git pull` | Refresh the active Git mirror from the current local Hub state | `neu git pull` |
| `neu git auth github-app --device` | Connect your GitHub App user account for Git mirror workflows | `neu git auth github-app --device` |

## Token Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neu token create --kind sync ...` | Create a short-lived sync token | `neu token create --kind sync --purpose backup --access both` |
| `neu token create --kind skills-upload ...` | Create a short-lived skills-upload token | `neu token create --kind skills-upload --purpose skills --platform claude-web` |

## Hosted Cloud And Remote Profiles

Use these commands when you want named remote profiles outside the bundle-oriented sync surface.

| Command | What it does | Example |
|---------|---------------|---------|
| `neu remote login <profile> [--url URL] [--token TOKEN]` | Log in to a named remote profile; `official` defaults to `https://neudrive.ai` | `neu remote login official` |
| `neu remote ls` | List saved remote profiles | `neu remote ls` |
| `neu remote use <profile>` | Switch the current profile | `neu remote use official` |
| `neu remote whoami <profile>` | Show current auth state for one profile | `neu remote whoami official` |
| `neu remote logout [profile]` | Clear the saved token for a profile | `neu remote logout official` |

## Bundle Sync Commands

Use `sync` when you want archive-style import/export flows against a remote neuDrive profile.

| Command | What it does | Example |
|---------|---------------|---------|
| `neu sync login --profile NAME` | Browser login and save a sync profile | `neu sync login --profile official` |
| `neu sync profiles` | List configured sync profiles | `neu sync profiles` |
| `neu sync use [--profile NAME \| NAME]` | Switch the active sync profile | `neu sync use official` |
| `neu sync whoami [--profile NAME]` | Show identity and scopes for the current sync profile | `neu sync whoami --profile official` |
| `neu sync logout --profile NAME` | Clear the saved sync token for one profile | `neu sync logout --profile official` |
| `neu sync export --source DIR [--format json\|archive] [--output FILE]` | Build an export bundle from a local source directory | `neu sync export --source ./skills --output backup.ndrv` |
| `neu sync preview --source DIR \| --bundle FILE` | Preview an incoming bundle without applying it | `neu sync preview --bundle backup.ndrv` |
| `neu sync push --source DIR \| --bundle FILE` | Push a source directory or bundle into a remote Hub | `neu sync push --bundle backup.ndrv` |
| `neu sync pull [--format json\|archive] [--output FILE]` | Pull content from a remote Hub into a local bundle file | `neu sync pull --format archive --output pulled.ndrvz` |
| `neu sync resume --bundle FILE [--session-file FILE]` | Resume an interrupted archive upload session | `neu sync resume --bundle backup.ndrvz` |
| `neu sync history` | Show recent sync sessions | `neu sync history` |
| `neu sync diff --left FILE --right FILE [--format text\|json]` | Compare two bundles and return non-zero when they differ | `neu sync diff --left before.ndrv --right after.ndrv` |

## Low-Level Server Commands

| Command | What it does | Example |
|---------|---------------|---------|
| `neu server [flags]` | Run the standalone neuDrive HTTP server | `neu server --listen 127.0.0.1:42690 --local-mode` |
| `neu mcp stdio [flags]` | Run the neuDrive MCP server over stdio | `neu mcp stdio --token-env NEUDRIVE_TOKEN` |

## Help

Use the built-in help when you need command-specific syntax:

```bash
neu help
neu help roots
neu help write
```

For testing coverage rather than day-to-day usage, see the [CLI test matrix](cli-test-matrix.md).
