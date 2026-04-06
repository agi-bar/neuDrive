# @agenthub/sdk

Agent Hub SDK for JavaScript/TypeScript.

The SDK targets the scoped-token `/agent/*` surface and supports both typed APIs
and the canonical virtual tree sync primitives (`snapshot` / `changes`).

## Quick Start

### With Scoped Token (Agent/MCP use)
```typescript
import { AgentHub } from '@agenthub/sdk'

const hub = new AgentHub({
  baseURL: 'https://hub.example.com',
  token: 'aht_xxxxx'
})

// Read user preferences
const profile = await hub.getProfile('preferences')

// Search memory
const results = await hub.searchMemory('海淀算力券')

// Sync a subtree
const snapshot = await hub.snapshot('/projects/my-project')
const delta = await hub.changes(snapshot.cursor, '/projects/my-project')

// Control a device
await hub.callDevice('living-room-light', 'off')
```

### With OAuth (Third-party app)
```typescript
import { AgentHubAuth } from '@agenthub/sdk'

const auth = new AgentHubAuth({
  baseURL: 'https://hub.example.com',
  clientId: 'your-client-id',
  clientSecret: 'your-client-secret'
})

// Redirect user to authorize
const url = auth.getAuthorizationURL('https://yourapp.com/callback', ['read:profile', 'read:memory'])

// After callback, exchange code
const { access_token, user } = await auth.exchangeCode(code, 'https://yourapp.com/callback')
```

## Installation

```bash
npm install @agenthub/sdk
```

## API Reference

### `AgentHub` (scoped token client)

| Method | Description |
|--------|-------------|
| `getProfile(category?)` | Get user profile entries |
| `updateProfile(category, content)` | Upsert a profile category |
| `searchMemory(query, scope?)` | Search memory/inbox |
| `listProjects()` | List all projects |
| `getProject(name)` | Get project with logs |
| `logAction(project, action, summary, tags?)` | Append project log |
| `listDirectory(path)` | List file tree directory |
| `readFile(path)` | Read a file |
| `writeFile(path, content, options?)` | Write a file with optional metadata / optimistic lock fields |
| `snapshot(path?)` | Fetch a subtree snapshot (`entries + cursor + root_checksum`) |
| `changes(cursor, path?)` | Fetch incremental subtree changes |
| `listSecrets()` | List vault scopes |
| `readSecret(scope)` | Read a vault secret |
| `listSkills()` | List indexed skill summaries from `SKILL.md` metadata |
| `readSkill(name)` | Read skill content |
| `listDevices()` | List registered devices |
| `callDevice(device, action, params?)` | Call a device action |
| `sendMessage(to, subject, body, opts?)` | Send an inbox message |
| `readInbox(role?, status?)` | Read inbox messages |
| `importSkill(name, files)` | Import a skill |
| `importClaudeMemory(memories)` | Import Claude memory |
| `importProfile(profile)` | Import profile fields |
| `exportAll()` | Export all user data |
| `getStats()` | Get dashboard statistics |
| `previewBundle(payload)` | Preview a JSON bundle or archive manifest |
| `importBundle(bundle)` | Import a V1 JSON bundle |
| `exportBundle(format, filters?)` | Export a JSON or archive bundle |
| `startSyncSession(request)` | Start an archive session |
| `resumeSession(sessionId, archive)` | Upload missing archive parts |
| `listSyncJobs()` | List sync history |

### Bundle Sync Example

```typescript
import { AgentHub } from '@agenthub/sdk'

const hub = new AgentHub({
  baseURL: 'https://hub.example.com',
  token: 'aht_xxxxx',
})

const preview = await hub.previewBundle({
  version: 'ahub.bundle/v1',
  created_at: new Date().toISOString(),
  mode: 'merge',
  profile: {},
  skills: {},
  memory: [],
})

const archive = await hub.exportBundle('archive')
const session = await hub.startSyncSession({
  transport_version: 'ahub.sync/v1',
  format: 'archive',
  mode: 'merge',
  manifest: { /* ... */ },
  archive_size_bytes: archive.length,
  archive_sha256: '...',
})
await hub.resumeSession(session.session_id, archive)
await hub.commitSession(session.session_id, preview.fingerprint)
```

For CLI and operational guidance, see [`docs/sync.md`](../../docs/sync.md).

### `AgentHubAuth` (OAuth helper)

| Method | Description |
|--------|-------------|
| `getAuthorizationURL(redirectURI, scopes)` | Build the OAuth authorization URL |
| `exchangeCode(code, redirectURI)` | Exchange auth code for tokens |
| `getUserInfo(accessToken)` | Get user info with an access token |

## License

MIT
