"""Agent Hub SDK client for Python."""

from __future__ import annotations

import httpx
from typing import Any, Optional

from .types import Device, ImportResult, InboxMessage, Profile, Project, TreeSnapshot, VaultScope


class AgentHub:
    """Synchronous Agent Hub SDK client.

    Use as a context manager to ensure the underlying HTTP connection is closed::

        with AgentHub("https://hub.example.com", token="aht_xxx") as hub:
            profile = hub.get_profile("preferences")
    """

    def __init__(self, base_url: str, token: str, timeout: float = 30.0) -> None:
        self.base_url = base_url.rstrip("/")
        self._client = httpx.Client(
            base_url=self.base_url,
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json",
            },
            timeout=timeout,
        )

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _request(self, method: str, path: str, **kwargs: Any) -> dict:
        resp = self._client.request(method, path, **kwargs)
        resp.raise_for_status()
        data = resp.json()
        if isinstance(data, dict) and data.get("ok") is True and "data" in data:
            return data["data"]
        return data

    @staticmethod
    def _file_path(path: str) -> str:
        return path if path.startswith("/") else f"/{path}"

    @classmethod
    def _dir_path(cls, path: str) -> str:
        safe_path = cls._file_path(path)
        if safe_path == "/":
            return safe_path
        return safe_path if safe_path.endswith("/") else f"{safe_path}/"

    # ------------------------------------------------------------------
    # Profile / Memory
    # ------------------------------------------------------------------

    def get_profile(self, category: Optional[str] = None) -> list[Profile]:
        """Retrieve profile entries, optionally filtered by *category*."""
        params: dict[str, str] = {}
        if category is not None:
            params["category"] = category
        data = self._request("GET", "/agent/memory/profile", params=params)
        raw = data.get("profiles") or []
        return [
            Profile(
                category=item.get("category", ""),
                content=item.get("content", ""),
                source=item.get("source", ""),
            )
            for item in raw
        ]

    def update_profile(self, category: str, content: str) -> None:
        """Create or update a profile entry for *category*."""
        self._request(
            "PUT",
            "/agent/memory/profile",
            json={"category": category, "content": content},
        )

    def search_memory(self, query: str, scope: str = "all") -> list[dict]:
        """Full-text search across memory / file tree."""
        data = self._request("GET", "/agent/search", params={"q": query, "scope": scope})
        return data.get("results") or []

    # ------------------------------------------------------------------
    # Projects
    # ------------------------------------------------------------------

    def list_projects(self) -> list[Project]:
        """Return all projects for the authenticated user."""
        data = self._request("GET", "/agent/projects")
        return [
            Project(
                name=p.get("name", ""),
                status=p.get("status", ""),
                context_md=p.get("context_md", ""),
            )
            for p in data.get("projects") or []
        ]

    def get_project(self, name: str) -> dict:
        """Get full project details including logs."""
        return self._request("GET", f"/agent/projects/{name}")

    def create_project(self, name: str) -> dict:
        """Create a new project."""
        return self._request("POST", "/agent/projects", json={"name": name})

    def log_action(
        self,
        project: str,
        action: str,
        summary: str,
        tags: Optional[list[str]] = None,
    ) -> None:
        """Append a log entry to a project."""
        payload: dict[str, Any] = {"action": action, "summary": summary}
        if tags:
            payload["tags"] = tags
        self._request("POST", f"/agent/projects/{project}/log", json=payload)

    # ------------------------------------------------------------------
    # File Tree
    # ------------------------------------------------------------------

    def list_directory(self, path: str = "/") -> list[dict]:
        """List entries under *path* in the virtual file tree."""
        data = self._request("GET", f"/agent/tree{self._dir_path(path)}")
        return data.get("children") or []

    def read_file(self, path: str) -> str:
        """Read a single file from the file tree and return its content."""
        data = self._request("GET", f"/agent/tree{self._file_path(path)}")
        return data.get("content", "")

    def write_file(self, path: str, content: str, **kwargs: Any) -> None:
        """Create or overwrite a file in the file tree."""
        self._request(
            "PUT",
            f"/agent/tree{self._file_path(path)}",
            json={
                "content": content,
                "content_type": kwargs.get("mime_type"),
                "metadata": kwargs.get("metadata"),
                "min_trust_level": kwargs.get("min_trust_level"),
                "expected_version": kwargs.get("expected_version"),
                "expected_checksum": kwargs.get("expected_checksum"),
            },
        )

    def snapshot(self, path: str = "/") -> TreeSnapshot:
        """Fetch a full subtree snapshot."""
        data = self._request("GET", "/agent/tree/snapshot", params={"path": path})
        return TreeSnapshot(
            path=data.get("path", path),
            cursor=data.get("cursor", 0),
            root_checksum=data.get("root_checksum", ""),
            entries=data.get("entries") or [],
        )

    def changes(self, cursor: int, path: str = "/") -> dict:
        """Fetch incremental subtree changes."""
        return self._request(
            "GET",
            "/agent/tree/changes",
            params={"cursor": str(cursor), "path": path},
        )

    # ------------------------------------------------------------------
    # Vault
    # ------------------------------------------------------------------

    def list_secrets(self) -> list[VaultScope]:
        """List available vault scopes (names only, not values)."""
        data = self._request("GET", "/agent/vault/scopes")
        scopes = data.get("scopes") or []
        return [
            VaultScope(scope=s, description="") if isinstance(s, str)
            else VaultScope(
                scope=s.get("scope", ""),
                description=s.get("description", ""),
                min_trust_level=s.get("min_trust_level", 4),
            )
            for s in scopes
        ]

    def read_secret(self, scope: str) -> str:
        """Read and decrypt a vault secret by scope name."""
        data = self._request("GET", f"/agent/vault/{scope}")
        return data.get("data", "")

    def write_secret(self, scope: str, value: str) -> None:
        """Write (encrypt and store) a vault secret."""
        self._request("PUT", f"/agent/vault/{scope}", json={"data": value})

    # ------------------------------------------------------------------
    # Skills
    # ------------------------------------------------------------------

    def list_skills(self) -> list[dict]:
        """List skill directories from the file tree."""
        data = self._request("GET", "/agent/skills")
        return data.get("skills") or []

    def read_skill(self, name: str) -> str:
        """Read the primary skill markdown file."""
        data = self._request("GET", f"/agent/tree/skills/{name}/SKILL.md")
        return data.get("content", "")

    # ------------------------------------------------------------------
    # Devices
    # ------------------------------------------------------------------

    def list_devices(self) -> list[Device]:
        """List all registered devices."""
        data = self._request("GET", "/agent/devices")
        return [
            Device(
                name=d.get("name", ""),
                device_type=d.get("type", d.get("device_type", "")),
                brand=d.get("brand", ""),
                protocol=d.get("protocol", ""),
                status=d.get("status", "online"),
            )
            for d in data.get("devices") or []
        ]

    def call_device(
        self, device: str, action: str, params: Optional[dict] = None
    ) -> dict:
        """Invoke an action on a registered device."""
        payload: dict[str, Any] = {"action": action}
        if params:
            payload["params"] = params
        return self._request("POST", f"/agent/devices/{device}/call", json=payload)

    # ------------------------------------------------------------------
    # Inbox
    # ------------------------------------------------------------------

    def send_message(self, to: str, subject: str, body: str, **kwargs: Any) -> None:
        """Send a message through the Hub inbox."""
        payload: dict[str, Any] = {"to": to, "subject": subject, "body": body}
        payload.update(kwargs)
        self._request("POST", "/agent/inbox/send", json=payload)

    def read_inbox(
        self, role: Optional[str] = None, status: str = "incoming"
    ) -> list[InboxMessage]:
        """Retrieve inbox messages for a given *role*."""
        role_path = role or "default"
        data = self._request(
            "GET", f"/agent/inbox/{role_path}", params={"status": status}
        )
        return [
            InboxMessage(
                id=m.get("id", ""),
                from_address=m.get("from_address", m.get("from", "")),
                to_address=m.get("to_address", m.get("to", "")),
                subject=m.get("subject", ""),
                body=m.get("body", ""),
                domain=m.get("domain", ""),
                action_type=m.get("action_type", ""),
                tags=m.get("tags") or [],
                status=m.get("status", "incoming"),
            )
            for m in data.get("messages") or []
        ]

    def archive_message(self, message_id: str) -> None:
        """Archive an inbox message by ID."""
        self._request("PUT", f"/agent/inbox/{message_id}/archive")

    # ------------------------------------------------------------------
    # Import / Export
    # ------------------------------------------------------------------

    def import_skill(self, name: str, files: dict[str, str]) -> ImportResult:
        """Import a skill as a set of files (relative_path -> content)."""
        data = self._request(
            "POST", "/agent/import/skill", json={"name": name, "files": files}
        )
        return self._parse_import_result(data)

    def import_claude_memory(self, memories: list[dict]) -> ImportResult:
        """Import memory entries from a Claude memory export."""
        data = self._request(
            "POST", "/agent/import/claude-memory", json={"memories": memories}
        )
        return self._parse_import_result(data)

    def import_profile(self, **kwargs: str) -> ImportResult:
        """Bulk-update profile categories (preferences, relationships, principles)."""
        data = self._request("POST", "/agent/import/profile", json=kwargs)
        return self._parse_import_result(data)

    def export_all(self) -> dict:
        """Export all Hub data as a JSON dict."""
        return self._request("GET", "/agent/export/all")

    @staticmethod
    def _parse_import_result(data: dict) -> ImportResult:
        """Normalise both legacy and v2 import response formats."""
        # v2 format: {"ok": true, "data": {"imported_count": N, ...}}
        inner = data.get("data", data)
        imported = inner.get("imported_count", inner.get("imported", 0))
        errors = inner.get("errors") or []
        return ImportResult(imported=imported, errors=errors or [])

    # ------------------------------------------------------------------
    # Stats / Dashboard
    # ------------------------------------------------------------------

    def get_stats(self) -> dict:
        """Retrieve dashboard statistics."""
        return self._request("GET", "/agent/dashboard/stats")

    # ------------------------------------------------------------------
    # Context manager
    # ------------------------------------------------------------------

    def __enter__(self) -> "AgentHub":
        return self

    def __exit__(self, *args: Any) -> None:
        self._client.close()

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self._client.close()


# ======================================================================
# Async variant
# ======================================================================


class AsyncAgentHub:
    """Asynchronous Agent Hub SDK client using ``httpx.AsyncClient``.

    Use as an async context manager::

        async with AsyncAgentHub("https://hub.example.com", token="aht_xxx") as hub:
            profile = await hub.get_profile("preferences")
    """

    def __init__(self, base_url: str, token: str, timeout: float = 30.0) -> None:
        self.base_url = base_url.rstrip("/")
        self._client = httpx.AsyncClient(
            base_url=self.base_url,
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json",
            },
            timeout=timeout,
        )

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    async def _request(self, method: str, path: str, **kwargs: Any) -> dict:
        resp = await self._client.request(method, path, **kwargs)
        resp.raise_for_status()
        data = resp.json()
        if isinstance(data, dict) and data.get("ok") is True and "data" in data:
            return data["data"]
        return data

    @staticmethod
    def _file_path(path: str) -> str:
        return path if path.startswith("/") else f"/{path}"

    @classmethod
    def _dir_path(cls, path: str) -> str:
        safe_path = cls._file_path(path)
        if safe_path == "/":
            return safe_path
        return safe_path if safe_path.endswith("/") else f"{safe_path}/"

    # ------------------------------------------------------------------
    # Profile / Memory
    # ------------------------------------------------------------------

    async def get_profile(self, category: Optional[str] = None) -> list[Profile]:
        params: dict[str, str] = {}
        if category is not None:
            params["category"] = category
        data = await self._request("GET", "/agent/memory/profile", params=params)
        raw = data.get("profiles") or []
        return [
            Profile(
                category=item.get("category", ""),
                content=item.get("content", ""),
                source=item.get("source", ""),
            )
            for item in raw
        ]

    async def update_profile(self, category: str, content: str) -> None:
        await self._request(
            "PUT",
            "/agent/memory/profile",
            json={"category": category, "content": content},
        )

    async def search_memory(self, query: str, scope: str = "all") -> list[dict]:
        data = await self._request(
            "GET", "/agent/search", params={"q": query, "scope": scope}
        )
        return data.get("results") or []

    # ------------------------------------------------------------------
    # Projects
    # ------------------------------------------------------------------

    async def list_projects(self) -> list[Project]:
        data = await self._request("GET", "/agent/projects")
        return [
            Project(
                name=p.get("name", ""),
                status=p.get("status", ""),
                context_md=p.get("context_md", ""),
            )
            for p in data.get("projects") or []
        ]

    async def get_project(self, name: str) -> dict:
        return await self._request("GET", f"/agent/projects/{name}")

    async def create_project(self, name: str) -> dict:
        return await self._request("POST", "/agent/projects", json={"name": name})

    async def log_action(
        self,
        project: str,
        action: str,
        summary: str,
        tags: Optional[list[str]] = None,
    ) -> None:
        payload: dict[str, Any] = {"action": action, "summary": summary}
        if tags:
            payload["tags"] = tags
        await self._request("POST", f"/agent/projects/{project}/log", json=payload)

    # ------------------------------------------------------------------
    # File Tree
    # ------------------------------------------------------------------

    async def list_directory(self, path: str = "/") -> list[dict]:
        data = await self._request("GET", f"/agent/tree{self._dir_path(path)}")
        return data.get("children") or []

    async def read_file(self, path: str) -> str:
        data = await self._request("GET", f"/agent/tree{self._file_path(path)}")
        return data.get("content", "")

    async def write_file(self, path: str, content: str, **kwargs: Any) -> None:
        await self._request(
            "PUT",
            f"/agent/tree{self._file_path(path)}",
            json={
                "content": content,
                "content_type": kwargs.get("mime_type"),
                "metadata": kwargs.get("metadata"),
                "min_trust_level": kwargs.get("min_trust_level"),
                "expected_version": kwargs.get("expected_version"),
                "expected_checksum": kwargs.get("expected_checksum"),
            },
        )

    async def snapshot(self, path: str = "/") -> TreeSnapshot:
        data = await self._request("GET", "/agent/tree/snapshot", params={"path": path})
        return TreeSnapshot(
            path=data.get("path", path),
            cursor=data.get("cursor", 0),
            root_checksum=data.get("root_checksum", ""),
            entries=data.get("entries") or [],
        )

    async def changes(self, cursor: int, path: str = "/") -> dict:
        return await self._request(
            "GET",
            "/agent/tree/changes",
            params={"cursor": str(cursor), "path": path},
        )

    # ------------------------------------------------------------------
    # Vault
    # ------------------------------------------------------------------

    async def list_secrets(self) -> list[VaultScope]:
        data = await self._request("GET", "/agent/vault/scopes")
        scopes = data.get("scopes") or []
        return [
            VaultScope(scope=s, description="") if isinstance(s, str)
            else VaultScope(
                scope=s.get("scope", ""),
                description=s.get("description", ""),
                min_trust_level=s.get("min_trust_level", 4),
            )
            for s in scopes
        ]

    async def read_secret(self, scope: str) -> str:
        data = await self._request("GET", f"/agent/vault/{scope}")
        return data.get("data", "")

    async def write_secret(self, scope: str, value: str) -> None:
        await self._request("PUT", f"/agent/vault/{scope}", json={"data": value})

    # ------------------------------------------------------------------
    # Skills
    # ------------------------------------------------------------------

    async def list_skills(self) -> list[dict]:
        data = await self._request("GET", "/agent/skills")
        return data.get("skills") or []

    async def read_skill(self, name: str) -> str:
        data = await self._request("GET", f"/agent/tree/skills/{name}/SKILL.md")
        return data.get("content", "")

    # ------------------------------------------------------------------
    # Devices
    # ------------------------------------------------------------------

    async def list_devices(self) -> list[Device]:
        data = await self._request("GET", "/agent/devices")
        return [
            Device(
                name=d.get("name", ""),
                device_type=d.get("type", d.get("device_type", "")),
                brand=d.get("brand", ""),
                protocol=d.get("protocol", ""),
                status=d.get("status", "online"),
            )
            for d in data.get("devices") or []
        ]

    async def call_device(
        self, device: str, action: str, params: Optional[dict] = None
    ) -> dict:
        payload: dict[str, Any] = {"action": action}
        if params:
            payload["params"] = params
        return await self._request(
            "POST", f"/agent/devices/{device}/call", json=payload
        )

    # ------------------------------------------------------------------
    # Inbox
    # ------------------------------------------------------------------

    async def send_message(
        self, to: str, subject: str, body: str, **kwargs: Any
    ) -> None:
        payload: dict[str, Any] = {"to": to, "subject": subject, "body": body}
        payload.update(kwargs)
        await self._request("POST", "/agent/inbox/send", json=payload)

    async def read_inbox(
        self, role: Optional[str] = None, status: str = "incoming"
    ) -> list[InboxMessage]:
        role_path = role or "default"
        data = await self._request(
            "GET", f"/agent/inbox/{role_path}", params={"status": status}
        )
        return [
            InboxMessage(
                id=m.get("id", ""),
                from_address=m.get("from_address", m.get("from", "")),
                to_address=m.get("to_address", m.get("to", "")),
                subject=m.get("subject", ""),
                body=m.get("body", ""),
                domain=m.get("domain", ""),
                action_type=m.get("action_type", ""),
                tags=m.get("tags") or [],
                status=m.get("status", "incoming"),
            )
            for m in data.get("messages") or []
        ]

    async def archive_message(self, message_id: str) -> None:
        await self._request("PUT", f"/agent/inbox/{message_id}/archive")

    # ------------------------------------------------------------------
    # Import / Export
    # ------------------------------------------------------------------

    async def import_skill(self, name: str, files: dict[str, str]) -> ImportResult:
        data = await self._request(
            "POST", "/agent/import/skill", json={"name": name, "files": files}
        )
        return self._parse_import_result(data)

    async def import_claude_memory(self, memories: list[dict]) -> ImportResult:
        data = await self._request(
            "POST", "/agent/import/claude-memory", json={"memories": memories}
        )
        return self._parse_import_result(data)

    async def import_profile(self, **kwargs: str) -> ImportResult:
        data = await self._request("POST", "/agent/import/profile", json=kwargs)
        return self._parse_import_result(data)

    async def export_all(self) -> dict:
        return await self._request("GET", "/agent/export/all")

    @staticmethod
    def _parse_import_result(data: dict) -> ImportResult:
        inner = data.get("data", data)
        imported = inner.get("imported_count", inner.get("imported", 0))
        errors = inner.get("errors") or []
        return ImportResult(imported=imported, errors=errors or [])

    # ------------------------------------------------------------------
    # Stats / Dashboard
    # ------------------------------------------------------------------

    async def get_stats(self) -> dict:
        return await self._request("GET", "/agent/dashboard/stats")

    # ------------------------------------------------------------------
    # Context manager
    # ------------------------------------------------------------------

    async def __aenter__(self) -> "AsyncAgentHub":
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self._client.aclose()

    async def close(self) -> None:
        """Close the underlying async HTTP client."""
        await self._client.aclose()
