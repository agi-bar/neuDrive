# `agenthub help`

Use this command when the user asks what Agent Hub can do from the current platform.

## Explain

- What Agent Hub MCP provides
- Which `agenthub` subcommands are available
- Which platform-native entrypoint to use
- Which portability manuals exist for deeper migration work
- That `agenthub git init [--output DIR]` can prepare a local Git repo mirror of the current local Hub data, excluding secrets
- That `agenthub git pull` refreshes that local mirror on demand
- That once the user has initialized that local mirror, later imports and writes keep syncing into the same directory automatically
