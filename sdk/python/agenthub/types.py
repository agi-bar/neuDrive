from dataclasses import dataclass, field
from typing import Optional


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
