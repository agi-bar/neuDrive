English | [简体中文](reference.zh-CN.md)

# neuDrive Reference

This document holds the longer reference material that was removed from the README so the top-level entry stays short.

## SDK

- [JavaScript SDK README](../sdk/javascript/README.md)
- [Python SDK README](../sdk/python/README.md)

JavaScript example:

```ts
import { NeuDrive } from '@agi-bar/neudrive'

const hub = new NeuDrive({
  baseURL: 'https://www.neudrive.ai',
  token: 'ndt_xxxxx',
})

const profile = await hub.getProfile('preferences')
await hub.sendMessage('worker:research@hub', 'Research Q2 policy', '...')
```

Python example:

```python
from neudrive import NeuDrive

with NeuDrive("https://www.neudrive.ai", token="ndt_xxxxx") as hub:
    profile = hub.get_profile("preferences")
    hub.send_message("worker:research@hub", "Research Q2 policy", "...")
```

## Core Capabilities

### 1. Unified Identity

One ID travels across agent platforms. neuDrive supports email/password sign-in, GitHub OAuth, and OAuth 2.0 provider flows for third-party apps.

### 2. Context Roaming

Three memory layers help context move with you:

- **Profile** for stable preferences and principles.
- **Projects** for project-specific context and structured logs.
- **Scratch** for short-lived working memory.

### 3. Secret Management

The vault stores sensitive data with encryption and trust-level controls so agents only see what they are allowed to use.

### 4. Capability Routing

`.skill` packages are registered once and exposed consistently so different agents can discover and use the same capabilities.

### 5. Agent Communication

Agents can send structured messages to each other, and those messages become searchable memory.

### 6. Device Control

Devices can be registered as skills so the hub can translate agent intent into concrete device actions.

## Self-Hosting And Local Development

### Docker Quick Start

```bash
cp neudrive.env.example neudrive.env
# Edit neudrive.env with your GitHub OAuth and secret settings
docker compose --env-file neudrive.env up
```

The service starts on `http://localhost:8080`.

### Local Development

```bash
# Backend
go run ./cmd/neudrive server --listen :8080

# Frontend (in another terminal)
cd web && npm install && npm run dev
```

Or use the Makefile:

```bash
make dev
make build
make test
```

## More Detailed Chinese Reference

For the current platform matrix, scoped token notes, architecture, admin console notes, security checklist, environment variables, and roadmap, see the Chinese reference:

- [Detailed Chinese Reference](reference.zh-CN.md)
