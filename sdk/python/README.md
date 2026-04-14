# neuDrive Python SDK

Python client library for [neuDrive](https://github.com/agi-bar/neudrive) -- AI identity and trust infrastructure.

The client wraps the scoped-token `/agent/*` API surface, including the
canonical virtual tree sync endpoints.

## Installation

```bash
pip install neudrive-sdk
```

Or install from source:

```bash
cd sdk/python
pip install -e .
```

## Quick Start

```python
from neudrive import NeuDrive

with NeuDrive("https://hub.example.com", token="ndt_xxx") as hub:
    # Read your profile
    profiles = hub.get_profile("preferences")
    for p in profiles:
        print(p.category, p.content)

    # Control a device
    hub.call_device("living-room-light", "off")

    # Sync a subtree
    snapshot = hub.snapshot("/projects/my-project")
    delta = hub.changes(snapshot.cursor, "/projects/my-project")

    # Send a message
    hub.send_message(to="assistant", subject="Hello", body="Testing the SDK")
```

## Async Usage

```python
import asyncio
from neudrive import AsyncNeuDrive

async def main():
    async with AsyncNeuDrive("https://hub.example.com", token="ndt_xxx") as hub:
        projects = await hub.list_projects()
        devices = await hub.list_devices()
        stats = await hub.get_stats()

asyncio.run(main())
```

## API Reference

### Profile and Memory

```python
hub.get_profile()                          # all profile entries
hub.get_profile("preferences")             # filtered by category
hub.update_profile("preferences", "...")   # upsert a category
hub.search_memory("query text")            # full-text search
```

### Projects

```python
hub.list_projects()
hub.get_project("my-project")
hub.create_project("new-project")
hub.log_action("my-project", "info", "deployed v2", tags=["deploy"])
```

### File Tree

```python
hub.list_directory("/")
hub.read_file("notes/todo.md")
hub.write_file("notes/todo.md", "# TODO\n- Ship SDK")
hub.write_file(
    "notes/todo.md",
    "# TODO\n- Ship SDK",
    expected_version=2,
    metadata={"source": "python-sdk"},
)
snapshot = hub.snapshot("/projects/my-project")
changes = hub.changes(snapshot.cursor, "/projects/my-project")
```

### Vault (Encrypted Secrets)

```python
hub.list_secrets()
hub.read_secret("api-keys")
hub.write_secret("api-keys", '{"openai": "sk-..."}')
```

### Skills

```python
hub.list_skills()
hub.read_skill("cyberzen-write")
# list_skills() returns indexed metadata such as description / when_to_use / tags
```

### Devices

```python
hub.list_devices()
hub.call_device("living-room-light", "on")
hub.call_device("thermostat", "set", params={"temperature": 22})
```

### Inbox

```python
hub.send_message(to="admin", subject="Alert", body="Disk full")
messages = hub.read_inbox(role="assistant")
hub.archive_message(messages[0].id)
```

### Import / Export

```python
hub.import_skill("my-skill", {"SKILL.md": "# My Skill\n..."})
hub.import_claude_memory([{"content": "User prefers dark mode", "type": "preference"}])
hub.import_profile(preferences="...", principles="...")
data = hub.export_all()
```

### Bundle Sync

```python
from neudrive import NeuDrive

with NeuDrive("https://hub.example.com", token="ndt_xxx") as hub:
    auth = hub.get_auth_info()
    print(auth["user_slug"], auth["scopes"])

    preview = hub.preview_bundle(bundle={"version": "ahub.bundle/v1", "created_at": "...", "mode": "merge", "skills": {}, "profile": {}, "memory": []})
    print(preview["fingerprint"])

    exported = hub.export_bundle("archive")
    session = hub.start_sync_session({
        "transport_version": "ahub.sync/v1",
        "format": "archive",
        "mode": "merge",
        "manifest": {...},
        "archive_size_bytes": len(exported),
        "archive_sha256": "...",
    })
    hub.resume_session(session.session_id, exported)
    hub.commit_session(session.session_id)

    jobs = hub.list_sync_jobs()
    print(jobs[0].status)
```

For CLI login/profile workflows and acceptance steps, see [`docs/sync.md`](../../docs/sync.md).

### Dashboard

```python
stats = hub.get_stats()
print(stats)  # {"connections": 3, "skills": 12, "devices": 2, "projects": 4, ...}
```

## OAuth for Third-Party Apps

```python
from neudrive import NeuDriveAuth

auth = NeuDriveAuth(
    base_url="https://hub.example.com",
    client_id="my-app",
    client_secret="secret",
)

# Step 1: redirect user
url = auth.get_authorization_url(
    redirect_uri="https://myapp.com/callback",
    scopes=["read:profile", "read:inbox"],
)

# Step 2: exchange code after redirect
tokens = auth.exchange_code(code="...", redirect_uri="https://myapp.com/callback")

# Step 3: fetch user info
user = auth.get_user_info(tokens["access_token"])
```

## Requirements

- Python >= 3.10
- httpx >= 0.25.0
