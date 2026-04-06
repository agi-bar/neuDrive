#!/usr/bin/env python3
"""
ahub-sync: V2 bundle sync helper for Agent Hub.

Examples:
  python3 tools/ahub-sync.py export --source /path/to/skills -o backup.ahub
  python3 tools/ahub-sync.py export --source /path/to/skills --format archive -o backup.ahubz
  python3 tools/ahub-sync.py preview --token aht_xxx --bundle backup.ahubz
  python3 tools/ahub-sync.py push --token aht_xxx --bundle backup.ahub --transport auto
  python3 tools/ahub-sync.py pull --token aht_xxx --format archive -o backup.ahubz
  python3 tools/ahub-sync.py resume --token aht_xxx --bundle backup.ahubz
  python3 tools/ahub-sync.py history --token aht_xxx
"""

from __future__ import annotations

import argparse
import base64
import copy
import hashlib
import io
import json
import mimetypes
import os
import sys
import time
import zipfile
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[1]
SDK_ROOT = REPO_ROOT / "sdk" / "python"
if str(SDK_ROOT) not in sys.path:
    sys.path.insert(0, str(SDK_ROOT))

from agenthub import AgentHub  # noqa: E402

DEFAULT_API_BASE = os.environ.get("AGENTHUB_API_BASE", "http://localhost:8080")
AUTO_THRESHOLD = 8 << 20
BINARY_EXTS = {
    ".png",
    ".jpg",
    ".jpeg",
    ".gif",
    ".pdf",
    ".zip",
    ".skill",
    ".bin",
    ".ico",
    ".woff",
    ".woff2",
    ".ttf",
}


def read_text_file(path: Path) -> str:
    for encoding in ("utf-8", "gbk", "latin-1"):
        try:
            return path.read_text(encoding=encoding)
        except UnicodeDecodeError:
            continue
    raise RuntimeError(f"unable to decode text file: {path}")


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def build_bundle(source_dir: str, mode: str) -> dict[str, Any]:
    source = Path(source_dir)
    if not source.is_dir():
        raise RuntimeError(f"source directory does not exist: {source}")

    bundle: dict[str, Any] = {
        "version": "ahub.bundle/v1",
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "source": "manual",
        "mode": mode,
        "profile": {},
        "skills": {},
        "memory": [],
        "stats": {
            "total_skills": 0,
            "total_files": 0,
            "total_bytes": 0,
            "binary_files": 0,
        },
    }

    for skill_dir in sorted(source.iterdir()):
        if not skill_dir.is_dir():
            continue
        skill = {"files": {}, "binary_files": {}}
        for file_path in sorted(skill_dir.rglob("*")):
            if not file_path.is_file():
                continue
            rel_path = str(file_path.relative_to(skill_dir)).replace("\\", "/")
            ext = file_path.suffix.lower()
            if ext in BINARY_EXTS:
                data = file_path.read_bytes()
                content_type = mimetypes.guess_type(file_path.name)[0] or "application/octet-stream"
                skill["binary_files"][rel_path] = {
                    "content_base64": base64.b64encode(data).decode("ascii"),
                    "content_type": content_type,
                    "size_bytes": len(data),
                    "sha256": sha256_hex(data),
                }
                bundle["stats"]["binary_files"] += 1
                bundle["stats"]["total_bytes"] += len(data)
            else:
                content = read_text_file(file_path)
                skill["files"][rel_path] = content
                bundle["stats"]["total_bytes"] += len(content.encode("utf-8"))
            bundle["stats"]["total_files"] += 1

        if "SKILL.md" not in skill["files"]:
            raise RuntimeError(f"skill {skill_dir.name} is missing SKILL.md")

        bundle["skills"][skill_dir.name] = skill
        bundle["stats"]["total_skills"] += 1

    return bundle


def apply_filters_to_bundle(bundle: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    include_domains = set(args.include_domain or [])
    include_skills = set(args.include_skill or [])
    exclude_skills = set(args.exclude_skill or [])

    if include_domains and "profile" not in include_domains:
        bundle["profile"] = {}
    if include_domains and "memory" not in include_domains:
        bundle["memory"] = []
    if include_domains and "skills" not in include_domains:
        bundle["skills"] = {}

    if bundle.get("skills"):
        filtered_skills: dict[str, Any] = {}
        for skill_name, skill in bundle["skills"].items():
            if include_skills and skill_name not in include_skills:
                continue
            if skill_name in exclude_skills:
                continue
            filtered_skills[skill_name] = skill
        bundle["skills"] = filtered_skills

    bundle["stats"] = calculate_bundle_stats(bundle)
    return bundle


def calculate_bundle_stats(bundle: dict[str, Any]) -> dict[str, int]:
    stats = {
        "total_skills": len(bundle.get("skills", {})),
        "total_files": 0,
        "total_bytes": 0,
        "binary_files": 0,
        "profile_items": len(bundle.get("profile", {})),
        "memory_items": len(bundle.get("memory", [])),
    }
    for content in bundle.get("profile", {}).values():
        stats["total_bytes"] += len(content.encode("utf-8"))
    for item in bundle.get("memory", []):
        stats["total_bytes"] += len((item.get("content") or "").encode("utf-8"))
    for skill in bundle.get("skills", {}).values():
        for content in skill.get("files", {}).values():
            stats["total_files"] += 1
            stats["total_bytes"] += len(content.encode("utf-8"))
        for blob in skill.get("binary_files", {}).values():
            stats["total_files"] += 1
            stats["binary_files"] += 1
            stats["total_bytes"] += int(blob.get("size_bytes") or 0)
    return stats


def print_bundle_stats(bundle: dict[str, Any]) -> None:
    stats = bundle.get("stats", {})
    print(
        f"Bundle: {stats.get('total_skills', 0)} skills, "
        f"{stats.get('total_files', 0)} files, "
        f"{stats.get('binary_files', 0)} binary, "
        f"{stats.get('total_bytes', 0)} bytes"
    )


def archive_entry_for_payload(archive_path: str, binary: bool, content_type: str, data: bytes) -> dict[str, Any]:
    return {
        "archive_path": archive_path,
        "binary": binary,
        "content_type": content_type,
        "size_bytes": len(data),
        "sha256": sha256_hex(data),
    }


def manifest_domains(bundle: dict[str, Any]) -> list[str]:
    domains: list[str] = []
    if bundle.get("profile"):
        domains.append("profile")
    if bundle.get("memory"):
        domains.append("memory")
    if bundle.get("skills"):
        domains.append("skills")
    return domains


def archive_entry_hash(entry: dict[str, Any]) -> str:
    return "|".join(
        [
            str(entry.get("archive_path", "")),
            "1" if entry.get("binary") else "0",
            str(entry.get("content_type", "")),
            str(entry.get("size_bytes", 0)),
            str(entry.get("sha256", "")),
        ]
    )


def archive_manifest_hash(manifest: dict[str, Any]) -> str:
    clean = dict(manifest)
    clean["archive_sha256"] = ""
    parts: list[str] = [
        str(clean.get("version", "")),
        str(clean.get("created_at", "")),
        str(clean.get("source", "")),
        str(clean.get("mode", "")),
        ",".join(sorted(clean.get("domains", []))),
        ",".join(sorted(clean.get("filters", {}).get("include_domains", []))),
        ",".join(sorted(clean.get("filters", {}).get("include_skills", []))),
        ",".join(sorted(clean.get("filters", {}).get("exclude_skills", []))),
    ]

    profile_files = clean.get("profile_files", {})
    for category in sorted(profile_files):
        parts.append(f"{category}={archive_entry_hash(profile_files[category])};")
    parts.append("|")

    for item in sorted(clean.get("memory_items", []), key=lambda item: item.get("id", "")):
        parts.append(
            "|".join(
                [
                    str(item.get("id", "")),
                    str(item.get("title", "")),
                    str(item.get("source", "")),
                    str(item.get("created_at", "")),
                    str(item.get("expires_at", "")),
                    str(item.get("archive_path", "")),
                    str(item.get("content_type", "")),
                    str(item.get("size_bytes", 0)),
                    str(item.get("sha256", "")),
                ]
            )
            + ";"
        )
    parts.append("|")

    skill_files = clean.get("skill_files", {})
    for skill_name in sorted(skill_files):
        parts.append(skill_name + "{")
        for rel_path in sorted(skill_files[skill_name]):
            parts.append(f"{rel_path}={archive_entry_hash(skill_files[skill_name][rel_path])};")
        parts.append("}")
    parts.append("|")

    stats = clean.get("stats", {})
    parts.append(
        "|".join(
            [
                str(stats.get("total_skills", 0)),
                str(stats.get("total_files", 0)),
                str(stats.get("total_bytes", 0)),
                str(stats.get("binary_files", 0)),
                str(stats.get("profile_items", 0)),
                str(stats.get("memory_items", 0)),
            ]
        )
    )
    return sha256_hex("".join(parts).encode("utf-8"))


def build_archive(bundle: dict[str, Any], filters: dict[str, Any]) -> tuple[bytes, dict[str, Any]]:
    payloads: dict[str, bytes] = {}
    manifest: dict[str, Any] = {
        "version": "ahub.bundle/v2",
        "created_at": bundle.get("created_at") or time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "source": bundle.get("source", "manual"),
        "mode": bundle.get("mode", "merge"),
        "domains": manifest_domains(bundle),
        "filters": filters,
        "profile_files": {},
        "memory_items": [],
        "skill_files": {},
        "stats": bundle.get("stats", calculate_bundle_stats(bundle)),
        "archive_sha256": "",
    }

    for category in sorted(bundle.get("profile", {})):
        data = bundle["profile"][category].encode("utf-8")
        archive_path = f"payload/profile/{category}.md"
        payloads[archive_path] = data
        manifest["profile_files"][category] = archive_entry_for_payload(archive_path, False, "text/markdown", data)

    for item in bundle.get("memory", []):
        created_at = item.get("created_at") or time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        memory_id = hashlib.sha1(f"{item.get('source', '')}:{item.get('title', '')}:{created_at}".encode("utf-8")).hexdigest()
        archive_path = f"payload/memory/{memory_id}.md"
        data = (item.get("content") or "").encode("utf-8")
        payloads[archive_path] = data
        manifest["memory_items"].append(
            {
                "id": memory_id,
                "title": item.get("title", ""),
                "source": item.get("source", ""),
                "created_at": created_at,
                "expires_at": item.get("expires_at", ""),
                "archive_path": archive_path,
                "content_type": "text/markdown",
                "size_bytes": len(data),
                "sha256": sha256_hex(data),
            }
        )

    for skill_name in sorted(bundle.get("skills", {})):
        manifest["skill_files"][skill_name] = {}
        skill = bundle["skills"][skill_name]
        for rel_path in sorted(skill.get("files", {})):
            data = skill["files"][rel_path].encode("utf-8")
            archive_path = f"payload/skills/{skill_name}/{rel_path}"
            payloads[archive_path] = data
            content_type = mimetypes.guess_type(rel_path)[0] or "text/plain"
            if rel_path.endswith(".md"):
                content_type = "text/markdown"
            manifest["skill_files"][skill_name][rel_path] = archive_entry_for_payload(archive_path, False, content_type, data)
        for rel_path in sorted(skill.get("binary_files", {})):
            blob = skill["binary_files"][rel_path]
            data = base64.b64decode(blob["content_base64"])
            archive_path = f"payload/skills/{skill_name}/{rel_path}"
            payloads[archive_path] = data
            manifest["skill_files"][skill_name][rel_path] = archive_entry_for_payload(
                archive_path,
                True,
                blob.get("content_type") or "application/octet-stream",
                data,
            )

    manifest["archive_sha256"] = archive_manifest_hash(manifest)
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("manifest.json", json.dumps(manifest, ensure_ascii=False, indent=2))
        for payload_path in sorted(payloads):
            zf.writestr(payload_path, payloads[payload_path])
    return buffer.getvalue(), manifest


def parse_archive(data: bytes) -> tuple[dict[str, Any], dict[str, Any]]:
    with zipfile.ZipFile(io.BytesIO(data), "r") as zf:  # type: ignore[name-defined]
        manifest = json.loads(zf.read("manifest.json").decode("utf-8"))
        if archive_manifest_hash(manifest) != manifest.get("archive_sha256"):
            raise RuntimeError("archive manifest hash mismatch")
        bundle: dict[str, Any] = {
            "version": "ahub.bundle/v1",
            "created_at": manifest.get("created_at"),
            "source": manifest.get("source", "manual"),
            "mode": manifest.get("mode", "merge"),
            "profile": {},
            "skills": {},
            "memory": [],
            "stats": manifest.get("stats", {}),
        }
        for category, entry in manifest.get("profile_files", {}).items():
            payload = zf.read(entry["archive_path"])
            bundle["profile"][category] = payload.decode("utf-8")
        for item in manifest.get("memory_items", []):
            payload = zf.read(item["archive_path"])
            bundle["memory"].append(
                {
                    "content": payload.decode("utf-8"),
                    "title": item.get("title", ""),
                    "source": item.get("source", ""),
                    "created_at": item.get("created_at", ""),
                    "expires_at": item.get("expires_at", ""),
                }
            )
        for skill_name, files in manifest.get("skill_files", {}).items():
            skill = {"files": {}, "binary_files": {}}
            for rel_path, entry in files.items():
                payload = zf.read(entry["archive_path"])
                if entry.get("binary"):
                    skill["binary_files"][rel_path] = {
                        "content_base64": base64.b64encode(payload).decode("ascii"),
                        "content_type": entry.get("content_type", "application/octet-stream"),
                        "size_bytes": len(payload),
                        "sha256": sha256_hex(payload),
                    }
                else:
                    skill["files"][rel_path] = payload.decode("utf-8")
            bundle["skills"][skill_name] = skill
        bundle["stats"] = calculate_bundle_stats(bundle)
        return bundle, manifest


def load_bundle_file(path: Path) -> tuple[dict[str, Any], dict[str, Any] | None]:
    data = path.read_bytes()
    if path.suffix == ".ahubz":
        bundle, manifest = parse_archive(data)
        return bundle, manifest
    try:
        bundle = json.loads(data.decode("utf-8"))
    except UnicodeDecodeError as exc:
        raise RuntimeError(f"unable to decode bundle file {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"invalid bundle json {path}: {exc}") from exc
    return bundle, None


def build_filters(args: argparse.Namespace) -> dict[str, Any]:
    return {
        "include_domains": args.include_domain or [],
        "include_skills": args.include_skill or [],
        "exclude_skills": args.exclude_skill or [],
    }


def default_session_file(bundle_path: Path) -> Path:
    return bundle_path.with_name(bundle_path.name + ".session.json")


def save_session_file(path: Path, payload: dict[str, Any]) -> None:
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def load_session_file(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def load_input_payload(args: argparse.Namespace) -> tuple[str, dict[str, Any] | None, bytes | None, dict[str, Any] | None, Path | None]:
    if getattr(args, "source", None):
        bundle = apply_filters_to_bundle(build_bundle(args.source, args.mode), args)
        return "bundle", bundle, None, None, None

    bundle_path = Path(args.bundle)
    bundle, manifest = load_bundle_file(bundle_path)
    if bundle_path.suffix == ".ahubz":
        data = bundle_path.read_bytes()
        return "archive", bundle, data, manifest, bundle_path
    bundle = apply_filters_to_bundle(bundle, args)
    return "bundle", bundle, None, None, bundle_path


def unwrap_import_result(result: dict[str, Any]) -> dict[str, Any]:
    return result.get("data", result) if isinstance(result, dict) else result


def _safe_label(value: str, fallback: str) -> str:
    normalized = "".join(ch.lower() if ch.isalnum() else "-" for ch in (value or "").strip())
    normalized = "-".join(part for part in normalized.split("-") if part)
    return normalized[:48] or fallback


def _memory_key(item: dict[str, Any]) -> tuple[str, str, str, str]:
    return (
        item.get("title", "") or "",
        item.get("source", "") or "",
        item.get("created_at", "") or "",
        item.get("expires_at", "") or "",
    )


def _memory_identity(item: dict[str, Any]) -> str:
    return json.dumps(
        {
            "title": item.get("title", "") or "",
            "source": item.get("source", "") or "",
            "created_at": item.get("created_at", "") or "",
            "expires_at": item.get("expires_at", "") or "",
            "content": item.get("content", "") or "",
        },
        ensure_ascii=False,
        sort_keys=True,
    )


def _memory_path(item: dict[str, Any]) -> str:
    title = _safe_label(item.get("title", ""), "memory")
    source = _safe_label(item.get("source", ""), "source")
    created = _safe_label(item.get("created_at", ""), "created")
    digest = sha256_hex(_memory_identity(item).encode("utf-8"))[:10]
    return f"/memory/diff/{source}/{created}-{title}-{digest}.md"


def _memory_group_path(key: tuple[str, str, str, str]) -> str:
    title, source, created_at, expires_at = key
    digest = sha256_hex("|".join([title, source, created_at, expires_at]).encode("utf-8"))[:10]
    return (
        f"/memory/diff/{_safe_label(source, 'source')}/"
        f"{_safe_label(created_at, 'created')}-{_safe_label(title, 'memory')}-{digest}.md"
    )


def normalize_bundle_for_diff(bundle: dict[str, Any], filters: dict[str, Any]) -> dict[str, Any]:
    working = copy.deepcopy(bundle)
    args = argparse.Namespace(
        include_domain=filters.get("include_domains") or [],
        include_skill=filters.get("include_skills") or [],
        exclude_skill=filters.get("exclude_skills") or [],
    )
    working = apply_filters_to_bundle(working, args)

    normalized_skills: dict[str, dict[str, Any]] = {}
    for skill_name, skill in sorted(working.get("skills", {}).items()):
        files: dict[str, Any] = {}
        for rel_path, content in sorted(skill.get("files", {}).items()):
            files[rel_path] = {
                "kind": "text",
                "content": content,
                "content_type": "text/markdown" if rel_path.endswith(".md") else (mimetypes.guess_type(rel_path)[0] or "text/plain"),
                "size_bytes": len(content.encode("utf-8")),
            }
        for rel_path, blob in sorted(skill.get("binary_files", {}).items()):
            data = base64.b64decode(blob["content_base64"])
            files[rel_path] = {
                "kind": "binary",
                "sha256": blob.get("sha256") or sha256_hex(data),
                "size_bytes": int(blob.get("size_bytes") or len(data)),
                "content_type": blob.get("content_type") or "application/octet-stream",
            }
        normalized_skills[skill_name] = files

    memory_groups: dict[tuple[str, str, str, str], list[dict[str, Any]]] = {}
    for item in working.get("memory", []):
        normalized_item = {
            "title": item.get("title", "") or "",
            "source": item.get("source", "") or "",
            "created_at": item.get("created_at", "") or "",
            "expires_at": item.get("expires_at", "") or "",
            "content": item.get("content", "") or "",
        }
        memory_groups.setdefault(_memory_key(normalized_item), []).append(normalized_item)
    for items in memory_groups.values():
        items.sort(key=_memory_identity)

    return {
        "profile": {category: working.get("profile", {})[category] for category in sorted(working.get("profile", {}))},
        "memory": memory_groups,
        "skills": normalized_skills,
    }


def _fresh_counts() -> dict[str, int]:
    return {"added": 0, "removed": 0, "changed": 0, "unchanged": 0}


def compare_bundles(left: dict[str, Any], right: dict[str, Any]) -> dict[str, Any]:
    result = {
        "equal": True,
        "summary": {
            "skills": _fresh_counts(),
            "files": _fresh_counts(),
            "profile": _fresh_counts(),
            "memory": _fresh_counts(),
        },
        "differences": [],
    }

    all_profile = sorted(set(left["profile"]) | set(right["profile"]))
    for category in all_profile:
        left_value = left["profile"].get(category)
        right_value = right["profile"].get(category)
        path = f"/memory/profile/{category}.md"
        if left_value is None:
            result["summary"]["profile"]["added"] += 1
            result["differences"].append({"domain": "profile", "path": path, "status": "added", "kind": "profile"})
            result["equal"] = False
        elif right_value is None:
            result["summary"]["profile"]["removed"] += 1
            result["differences"].append({"domain": "profile", "path": path, "status": "removed", "kind": "profile"})
            result["equal"] = False
        elif left_value == right_value:
            result["summary"]["profile"]["unchanged"] += 1
        else:
            result["summary"]["profile"]["changed"] += 1
            result["differences"].append(
                {
                    "domain": "profile",
                    "path": path,
                    "status": "changed",
                    "kind": "profile",
                    "details": {
                        "left_bytes": len(left_value.encode("utf-8")),
                        "right_bytes": len(right_value.encode("utf-8")),
                    },
                }
            )
            result["equal"] = False

    all_memory = sorted(set(left["memory"]) | set(right["memory"]))
    for key in all_memory:
        left_items = left["memory"].get(key, [])
        right_items = right["memory"].get(key, [])
        path = _memory_group_path(key)
        left_identities = [_memory_identity(item) for item in left_items]
        right_identities = [_memory_identity(item) for item in right_items]
        if not left_items:
            result["summary"]["memory"]["added"] += len(right_items)
            for item in right_items:
                result["differences"].append({"domain": "memory", "path": _memory_path(item), "status": "added", "kind": "memory"})
            result["equal"] = False
        elif not right_items:
            result["summary"]["memory"]["removed"] += len(left_items)
            for item in left_items:
                result["differences"].append({"domain": "memory", "path": _memory_path(item), "status": "removed", "kind": "memory"})
            result["equal"] = False
        elif left_identities == right_identities:
            result["summary"]["memory"]["unchanged"] += len(left_items)
        else:
            result["summary"]["memory"]["changed"] += max(len(left_items), len(right_items))
            result["differences"].append(
                {
                    "domain": "memory",
                    "path": path,
                    "status": "changed",
                    "kind": "memory",
                    "details": {
                        "left_items": len(left_items),
                        "right_items": len(right_items),
                    },
                }
            )
            result["equal"] = False

    all_skills = sorted(set(left["skills"]) | set(right["skills"]))
    for skill_name in all_skills:
        left_files = left["skills"].get(skill_name)
        right_files = right["skills"].get(skill_name)
        skill_status = "unchanged"
        if left_files is None:
            skill_status = "added"
            result["summary"]["skills"]["added"] += 1
            result["equal"] = False
        elif right_files is None:
            skill_status = "removed"
            result["summary"]["skills"]["removed"] += 1
            result["equal"] = False

        all_files = sorted(set((left_files or {}).keys()) | set((right_files or {}).keys()))
        for rel_path in all_files:
            path = f"/skills/{skill_name}/{rel_path}"
            left_entry = (left_files or {}).get(rel_path)
            right_entry = (right_files or {}).get(rel_path)
            if left_entry is None:
                result["summary"]["files"]["added"] += 1
                result["differences"].append({"domain": "skills", "path": path, "status": "added", "kind": right_entry["kind"]})
                if skill_status == "unchanged":
                    skill_status = "changed"
                result["equal"] = False
                continue
            if right_entry is None:
                result["summary"]["files"]["removed"] += 1
                result["differences"].append({"domain": "skills", "path": path, "status": "removed", "kind": left_entry["kind"]})
                if skill_status == "unchanged":
                    skill_status = "changed"
                result["equal"] = False
                continue

            if left_entry["kind"] == "text" and right_entry["kind"] == "text":
                if left_entry["content"] == right_entry["content"]:
                    result["summary"]["files"]["unchanged"] += 1
                else:
                    result["summary"]["files"]["changed"] += 1
                    result["differences"].append(
                        {
                            "domain": "skills",
                            "path": path,
                            "status": "changed",
                            "kind": "text",
                            "details": {
                                "left_bytes": left_entry["size_bytes"],
                                "right_bytes": right_entry["size_bytes"],
                            },
                        }
                    )
                    if skill_status == "unchanged":
                        skill_status = "changed"
                    result["equal"] = False
                continue

            if left_entry["kind"] == "binary" and right_entry["kind"] == "binary":
                if (
                    left_entry["sha256"] == right_entry["sha256"]
                    and left_entry["size_bytes"] == right_entry["size_bytes"]
                    and left_entry["content_type"] == right_entry["content_type"]
                ):
                    result["summary"]["files"]["unchanged"] += 1
                else:
                    result["summary"]["files"]["changed"] += 1
                    result["differences"].append(
                        {
                            "domain": "skills",
                            "path": path,
                            "status": "changed",
                            "kind": "binary",
                            "details": {
                                "left_sha256": left_entry["sha256"],
                                "right_sha256": right_entry["sha256"],
                                "left_size_bytes": left_entry["size_bytes"],
                                "right_size_bytes": right_entry["size_bytes"],
                                "left_content_type": left_entry["content_type"],
                                "right_content_type": right_entry["content_type"],
                            },
                        }
                    )
                    if skill_status == "unchanged":
                        skill_status = "changed"
                    result["equal"] = False
                continue

            result["summary"]["files"]["changed"] += 1
            result["differences"].append(
                {
                    "domain": "skills",
                    "path": path,
                    "status": "changed",
                    "kind": "file",
                    "details": {
                        "left_kind": left_entry["kind"],
                        "right_kind": right_entry["kind"],
                    },
                }
            )
            if skill_status == "unchanged":
                skill_status = "changed"
            result["equal"] = False

        if skill_status == "unchanged":
            result["summary"]["skills"]["unchanged"] += 1
        elif skill_status == "changed":
            result["summary"]["skills"]["changed"] += 1

    result["differences"].sort(key=lambda item: (item["domain"], item["status"], item["path"]))
    return result


def render_diff_text(diff: dict[str, Any], left_label: str, right_label: str) -> str:
    lines = [
        f"Diff: {left_label} -> {right_label}",
        f"Equal: {'yes' if diff['equal'] else 'no'}",
        "",
        "Summary:",
    ]
    for section in ("skills", "files", "profile", "memory"):
        counts = diff["summary"][section]
        lines.append(
            f"  {section}: added={counts['added']} removed={counts['removed']} changed={counts['changed']} unchanged={counts['unchanged']}"
        )
    lines.append("")
    lines.append("Differences:")
    if not diff["differences"]:
        lines.append("  none")
    else:
        for item in diff["differences"]:
            detail = ""
            if item.get("details"):
                compact = []
                for key in sorted(item["details"]):
                    compact.append(f"{key}={item['details'][key]}")
                detail = " (" + ", ".join(compact) + ")"
            lines.append(f"  [{item['status']}] {item['path']} [{item.get('kind', item['domain'])}]{detail}")
    return "\n".join(lines)


def cmd_export(args: argparse.Namespace) -> int:
    bundle = apply_filters_to_bundle(build_bundle(args.source, args.mode), args)
    print_bundle_stats(bundle)
    filters = build_filters(args)
    output = Path(args.output)
    if args.format == "archive":
        archive, manifest = build_archive(bundle, filters)
        output.write_bytes(archive)
        print(json.dumps({"manifest": manifest, "bytes": len(archive)}, ensure_ascii=False, indent=2))
    else:
        output.write_text(json.dumps(bundle, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"saved export to {output}")
    return 0


def cmd_preview(args: argparse.Namespace) -> int:
    kind, bundle, _, manifest, _ = load_input_payload(args)
    with AgentHub(args.api_base, args.token) as hub:
        result = hub.preview_bundle(bundle=bundle if kind == "bundle" else None, manifest=manifest if kind == "archive" else None)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


def cmd_push(args: argparse.Namespace) -> int:
    kind, bundle, archive_bytes, manifest, bundle_path = load_input_payload(args)
    filters = build_filters(args)
    with AgentHub(args.api_base, args.token) as hub:
        transport = args.transport
        if transport == "auto":
            if kind == "archive":
                transport = "archive"
            else:
                encoded = json.dumps(bundle, ensure_ascii=False).encode("utf-8")
                transport = "json" if len(encoded) <= AUTO_THRESHOLD else "archive"

        if transport == "json":
            if bundle is None:
                raise RuntimeError("json transport requires a JSON bundle or source directory")
            result = hub.import_bundle(bundle)
            print(json.dumps(unwrap_import_result(result), ensure_ascii=False, indent=2))
            return 0

        if archive_bytes is None or manifest is None:
            if bundle is None:
                raise RuntimeError("archive transport requires a bundle or archive file")
            archive_bytes, manifest = build_archive(bundle, filters)

        if bundle_path is None or bundle_path.suffix != ".ahubz":
            stem = Path(args.source).name if getattr(args, "source", None) else (bundle_path.stem if bundle_path else "bundle")
            archive_path = Path(f"{stem}.ahubz")
            archive_path.write_bytes(archive_bytes)
            bundle_path = archive_path

        session = hub.start_sync_session(
            {
                "transport_version": "ahub.sync/v1",
                "format": "archive",
                "mode": manifest.get("mode", args.mode),
                "manifest": manifest,
                "archive_size_bytes": len(archive_bytes),
                "archive_sha256": manifest["archive_sha256"],
            }
        )
        session_file = Path(args.session_file) if args.session_file else default_session_file(bundle_path or Path(args.bundle or "bundle.ahubz"))
        save_session_file(
            session_file,
            {
                "api_base": args.api_base,
                "bundle_path": str(bundle_path),
                "session_id": session.session_id,
                "preview_fingerprint": "",
                "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            },
        )

        state = hub.resume_session(session.session_id, archive_bytes)
        if state.status != "ready":
            state = hub.get_sync_session(session.session_id)
        result = hub.commit_session(session.session_id)
        session_file.unlink(missing_ok=True)
        print(json.dumps(unwrap_import_result(result), ensure_ascii=False, indent=2))
    return 0


def cmd_pull(args: argparse.Namespace) -> int:
    filters = build_filters(args)
    with AgentHub(args.api_base, args.token) as hub:
        exported = hub.export_bundle(args.format, filters)
    output = Path(args.output)
    if args.format == "archive":
        output.write_bytes(bytes(exported))
        print(f"saved archive to {output} ({len(exported)} bytes)")
    else:
        output.write_text(json.dumps(exported, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print_bundle_stats(exported)
        print(f"saved bundle to {output}")
    return 0


def cmd_resume(args: argparse.Namespace) -> int:
    session_file = Path(args.session_file) if args.session_file else default_session_file(Path(args.bundle))
    state = load_session_file(session_file)
    bundle_path = Path(state["bundle_path"])
    archive_bytes = bundle_path.read_bytes()
    with AgentHub(args.api_base, args.token) as hub:
        session = hub.resume_session(state["session_id"], archive_bytes)
        if session.status != "ready":
            session = hub.get_sync_session(state["session_id"])
        result = hub.commit_session(state["session_id"], state.get("preview_fingerprint") or None)
    session_file.unlink(missing_ok=True)
    print(json.dumps({"session": session.__dict__, "result": unwrap_import_result(result)}, ensure_ascii=False, indent=2))
    return 0


def cmd_history(args: argparse.Namespace) -> int:
    with AgentHub(args.api_base, args.token) as hub:
        jobs = hub.list_sync_jobs()
    print(json.dumps([job.__dict__ for job in jobs], ensure_ascii=False, indent=2))
    return 0


def cmd_diff(args: argparse.Namespace) -> int:
    filters = build_filters(args)
    try:
        left_bundle, _ = load_bundle_file(Path(args.left))
        right_bundle, _ = load_bundle_file(Path(args.right))
    except (OSError, RuntimeError, zipfile.BadZipFile, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 2

    left = normalize_bundle_for_diff(left_bundle, filters)
    right = normalize_bundle_for_diff(right_bundle, filters)
    diff = compare_bundles(left, right)
    if args.format == "json":
        print(json.dumps(diff, ensure_ascii=False, indent=2))
    else:
        print(render_diff_text(diff, args.left, args.right))
    return 0 if diff["equal"] else 1


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Agent Hub bundle sync helper")
    sub = parser.add_subparsers(dest="command", required=True)

    export_cmd = sub.add_parser("export", help="build a local bundle or archive from a skills directory")
    export_cmd.add_argument("--source", required=True, help="directory containing skill subdirectories")
    export_cmd.add_argument("--mode", default="merge", choices=("merge", "mirror"))
    export_cmd.add_argument("--format", default="json", choices=("json", "archive"))
    export_cmd.add_argument("--include-domain", action="append", choices=("profile", "memory", "skills"))
    export_cmd.add_argument("--include-skill", action="append")
    export_cmd.add_argument("--exclude-skill", action="append")
    export_cmd.add_argument("-o", "--output", default="backup.ahub")
    export_cmd.set_defaults(func=cmd_export)

    push_cmd = sub.add_parser("push", help="push a bundle or archive into Agent Hub")
    push_cmd.add_argument("--token", required=True)
    push_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    push_group = push_cmd.add_mutually_exclusive_group(required=True)
    push_group.add_argument("--source", help="directory containing skill subdirectories")
    push_group.add_argument("--bundle", help="existing .ahub or .ahubz bundle file")
    push_cmd.add_argument("--mode", default="merge", choices=("merge", "mirror"))
    push_cmd.add_argument("--transport", default="auto", choices=("auto", "json", "archive"))
    push_cmd.add_argument("--include-domain", action="append", choices=("profile", "memory", "skills"))
    push_cmd.add_argument("--include-skill", action="append")
    push_cmd.add_argument("--exclude-skill", action="append")
    push_cmd.add_argument("--session-file")
    push_cmd.set_defaults(func=cmd_push)

    preview_cmd = sub.add_parser("preview", help="preview bundle changes before importing into Agent Hub")
    preview_cmd.add_argument("--token", required=True)
    preview_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    preview_group = preview_cmd.add_mutually_exclusive_group(required=True)
    preview_group.add_argument("--source", help="directory containing skill subdirectories")
    preview_group.add_argument("--bundle", help="existing .ahub or .ahubz bundle file")
    preview_cmd.add_argument("--mode", default="merge", choices=("merge", "mirror"))
    preview_cmd.add_argument("--include-domain", action="append", choices=("profile", "memory", "skills"))
    preview_cmd.add_argument("--include-skill", action="append")
    preview_cmd.add_argument("--exclude-skill", action="append")
    preview_cmd.set_defaults(func=cmd_preview)

    pull_cmd = sub.add_parser("pull", help="export a bundle from Agent Hub")
    pull_cmd.add_argument("--token", required=True)
    pull_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    pull_cmd.add_argument("--format", default="json", choices=("json", "archive"))
    pull_cmd.add_argument("--include-domain", action="append", choices=("profile", "memory", "skills"))
    pull_cmd.add_argument("--include-skill", action="append")
    pull_cmd.add_argument("--exclude-skill", action="append")
    pull_cmd.add_argument("-o", "--output", default="backup.ahub")
    pull_cmd.set_defaults(func=cmd_pull)

    resume_cmd = sub.add_parser("resume", help="resume an in-flight archive upload using the sidecar session file")
    resume_cmd.add_argument("--token", required=True)
    resume_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    resume_cmd.add_argument("--bundle", required=True, help="existing .ahubz archive bundle file")
    resume_cmd.add_argument("--session-file")
    resume_cmd.set_defaults(func=cmd_resume)

    history_cmd = sub.add_parser("history", help="show sync import/export history")
    history_cmd.add_argument("--token", required=True)
    history_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    history_cmd.set_defaults(func=cmd_history)

    diff_cmd = sub.add_parser("diff", help="compare two bundle/archive files")
    diff_cmd.add_argument("--left", required=True, help="left bundle file (.ahub or .ahubz)")
    diff_cmd.add_argument("--right", required=True, help="right bundle file (.ahub or .ahubz)")
    diff_cmd.add_argument("--include-domain", action="append", choices=("profile", "memory", "skills"))
    diff_cmd.add_argument("--include-skill", action="append")
    diff_cmd.add_argument("--exclude-skill", action="append")
    diff_cmd.add_argument("--format", default="text", choices=("text", "json"))
    diff_cmd.set_defaults(func=cmd_diff)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        return args.func(args)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
