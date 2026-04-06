from dataclasses import dataclass, field
from typing import Any, Optional


@dataclass
class User:
    id: str
    slug: str
    display_name: str
    email: str = ""
    timezone: str = "UTC"
    language: str = "zh-CN"


@dataclass
class Profile:
    category: str
    content: str
    source: str = ""


@dataclass
class Project:
    name: str
    status: str
    context_md: str = ""


@dataclass
class VaultScope:
    scope: str
    description: str
    min_trust_level: int = 4


@dataclass
class InboxMessage:
    id: str
    from_address: str
    to_address: str
    subject: str
    body: str
    domain: str = ""
    action_type: str = ""
    tags: list[str] = field(default_factory=list)
    status: str = "incoming"


@dataclass
class Device:
    name: str
    device_type: str
    brand: str = ""
    protocol: str = ""
    status: str = "online"


@dataclass
class ImportResult:
    imported: int
    errors: list[str] = field(default_factory=list)


@dataclass
class FileTreeEntry:
    path: str
    name: str = ""
    is_dir: bool = False
    kind: str = ""
    content: str = ""
    mime_type: str = ""
    version: int = 0
    checksum: str = ""
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class TreeChange:
    cursor: int
    change_type: str
    entry: dict[str, Any]


@dataclass
class TreeSnapshot:
    path: str
    cursor: int
    root_checksum: str
    entries: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class BundleFilters:
    include_domains: list[str] = field(default_factory=list)
    include_skills: list[str] = field(default_factory=list)
    exclude_skills: list[str] = field(default_factory=list)


@dataclass
class SyncSessionStatus:
    session_id: str
    job_id: str
    status: str
    chunk_size_bytes: int
    total_parts: int
    expires_at: str
    mode: str = "merge"
    summary: dict[str, Any] = field(default_factory=dict)
    received_parts: list[int] = field(default_factory=list)
    missing_parts: list[int] = field(default_factory=list)


@dataclass
class SyncJob:
    id: str
    user_id: str
    direction: str
    transport: str
    status: str
    source: str = ""
    mode: str = "merge"
    filters: dict[str, Any] = field(default_factory=dict)
    summary: dict[str, Any] = field(default_factory=dict)
    error: str = ""
    created_at: str = ""
    updated_at: str = ""
    completed_at: Optional[str] = None
