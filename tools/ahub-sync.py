#!/usr/bin/env python3
"""
ahub-sync: export/push/pull Agent Hub bundles.

Examples:
  python3 tools/ahub-sync.py export --source /path/to/skills -o backup.ahub
  python3 tools/ahub-sync.py preview --token aht_xxx --bundle backup.ahub
  python3 tools/ahub-sync.py push --token aht_xxx --bundle backup.ahub --api-base https://hub.example.com
  python3 tools/ahub-sync.py push --token aht_xxx --source /path/to/skills --mode mirror
  python3 tools/ahub-sync.py pull --token aht_xxx -o backup.ahub
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import mimetypes
import os
import sys
import time
from pathlib import Path
from typing import Any
from urllib import error, request


DEFAULT_API_BASE = os.environ.get("AGENTHUB_API_BASE", "http://localhost:8080")
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


def http_json(method: str, url: str, token: str | None = None, payload: dict[str, Any] | None = None, timeout: int = 120) -> dict[str, Any]:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8") if payload is not None else None
    req = request.Request(url, data=body, headers=headers, method=method)
    try:
        with request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            parsed = json.loads(raw)
        except Exception:
            parsed = {"error": raw}
        raise RuntimeError(f"HTTP {exc.code}: {parsed}") from exc


def read_text_file(path: Path) -> str:
    for encoding in ("utf-8", "gbk", "latin-1"):
        try:
            return path.read_text(encoding=encoding)
        except UnicodeDecodeError:
            continue
    raise RuntimeError(f"unable to decode text file: {path}")


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
                sha256 = hashlib.sha256(data).hexdigest()
                content_type = mimetypes.guess_type(file_path.name)[0] or "application/octet-stream"
                skill["binary_files"][rel_path] = {
                    "content_base64": base64.b64encode(data).decode("ascii"),
                    "content_type": content_type,
                    "size_bytes": len(data),
                    "sha256": sha256,
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


def print_bundle_stats(bundle: dict[str, Any]) -> None:
    stats = bundle.get("stats", {})
    print(
        f"Bundle: {stats.get('total_skills', 0)} skills, "
        f"{stats.get('total_files', 0)} files, "
        f"{stats.get('binary_files', 0)} binary, "
        f"{stats.get('total_bytes', 0)} bytes"
    )


def unwrap_response(response: dict[str, Any]) -> dict[str, Any]:
    if response.get("ok") is True and "data" in response:
        return response["data"]
    return response


def cmd_export(args: argparse.Namespace) -> int:
    bundle = build_bundle(args.source, args.mode)
    print_bundle_stats(bundle)
    with open(args.output, "w", encoding="utf-8") as fh:
        json.dump(bundle, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    print(f"saved bundle to {args.output}")
    return 0


def cmd_push(args: argparse.Namespace) -> int:
    if args.bundle:
        with open(args.bundle, "r", encoding="utf-8") as fh:
            bundle = json.load(fh)
    else:
        bundle = build_bundle(args.source, args.mode)

    if args.mode:
        bundle["mode"] = args.mode

    print_bundle_stats(bundle)
    response = http_json(
        "POST",
        f"{args.api_base.rstrip('/')}/agent/import/bundle",
        token=args.token,
        payload=bundle,
    )
    result = unwrap_response(response)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


def cmd_preview(args: argparse.Namespace) -> int:
    if args.bundle:
        with open(args.bundle, "r", encoding="utf-8") as fh:
            bundle = json.load(fh)
    else:
        bundle = build_bundle(args.source, args.mode)

    if args.mode:
        bundle["mode"] = args.mode

    print_bundle_stats(bundle)
    response = http_json(
        "POST",
        f"{args.api_base.rstrip('/')}/agent/import/preview",
        token=args.token,
        payload=bundle,
    )
    result = unwrap_response(response)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


def cmd_pull(args: argparse.Namespace) -> int:
    response = http_json(
        "GET",
        f"{args.api_base.rstrip('/')}/agent/export/bundle",
        token=args.token,
    )
    bundle = unwrap_response(response)
    with open(args.output, "w", encoding="utf-8") as fh:
        json.dump(bundle, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    print_bundle_stats(bundle)
    print(f"saved bundle to {args.output}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Agent Hub bundle sync helper")
    sub = parser.add_subparsers(dest="command", required=True)

    export_cmd = sub.add_parser("export", help="build a local .ahub bundle from a skills directory")
    export_cmd.add_argument("--source", required=True, help="directory containing skill subdirectories")
    export_cmd.add_argument("--mode", default="merge", choices=("merge", "mirror"))
    export_cmd.add_argument("-o", "--output", default="backup.ahub")
    export_cmd.set_defaults(func=cmd_export)

    push_cmd = sub.add_parser("push", help="push a bundle into Agent Hub")
    push_cmd.add_argument("--token", required=True)
    push_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    push_group = push_cmd.add_mutually_exclusive_group(required=True)
    push_group.add_argument("--source", help="directory containing skill subdirectories")
    push_group.add_argument("--bundle", help="existing .ahub bundle file")
    push_cmd.add_argument("--mode", default="merge", choices=("merge", "mirror"))
    push_cmd.set_defaults(func=cmd_push)

    preview_cmd = sub.add_parser("preview", help="preview bundle changes before importing into Agent Hub")
    preview_cmd.add_argument("--token", required=True)
    preview_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    preview_group = preview_cmd.add_mutually_exclusive_group(required=True)
    preview_group.add_argument("--source", help="directory containing skill subdirectories")
    preview_group.add_argument("--bundle", help="existing .ahub bundle file")
    preview_cmd.add_argument("--mode", default="merge", choices=("merge", "mirror"))
    preview_cmd.set_defaults(func=cmd_preview)

    pull_cmd = sub.add_parser("pull", help="export a bundle from Agent Hub")
    pull_cmd.add_argument("--token", required=True)
    pull_cmd.add_argument("--api-base", default=DEFAULT_API_BASE)
    pull_cmd.add_argument("-o", "--output", default="backup.ahub")
    pull_cmd.set_defaults(func=cmd_pull)

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
