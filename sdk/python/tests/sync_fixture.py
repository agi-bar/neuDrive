from __future__ import annotations

import hashlib
import json
import os
import subprocess
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
FIXTURE_DIR = ROOT / "internal" / "services" / "testdata"


def _load_skill_fixture() -> dict[str, str]:
    with zipfile.ZipFile(FIXTURE_DIR / "ahub-sync.skill") as zf:
        files: dict[str, str] = {}
        for name in zf.namelist():
            if not name.startswith("pkg-skill/") or name.endswith("/"):
                continue
            rel = name.removeprefix("pkg-skill/")
            files[rel] = zf.read(name).decode("utf-8")
        return files


def _load_plan() -> dict:
    return json.loads((FIXTURE_DIR / "sync-fixture-plan.json").read_text("utf-8"))


def _load_binary() -> bytes:
    return (FIXTURE_DIR / "tiny.png").read_bytes()


def _expanded_binary(base: bytes, seed: str, multiplier: int) -> bytes:
    target_size = len(base) + max(multiplier, 1) * (256 << 10)
    payload = bytearray(base)
    counter = 0
    while len(payload) < target_size:
        payload.extend(hashlib.sha256(f"{seed}:{counter}".encode("utf-8")).digest())
        counter += 1
    return bytes(payload[:target_size])


def materialize_source(multiplier: int = 1) -> str:
    files = _load_skill_fixture()
    plan = _load_plan()
    binary = _load_binary()
    tempdir = Path(tempfile.mkdtemp(prefix="agenthub-sync-fixture-"))

    for skill_name in plan["skill_names"]:
        skill_root = tempdir / skill_name
        skill_root.mkdir(parents=True, exist_ok=True)
        for rel_path, content in files.items():
            target = skill_root / rel_path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content, encoding="utf-8")
        for extra in plan["extra_text_files"]:
            target = skill_root / extra["path"]
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text((files[extra["source"]] + "\n") * (extra["repeat"] * max(multiplier, 1)), encoding="utf-8")
        for rel_path in plan["binary_assignments"].get(skill_name, []):
            target = skill_root / rel_path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(_expanded_binary(binary, f"{skill_name}:{rel_path}", multiplier))
    return str(tempdir)


def _agenthub_cli_command() -> list[str]:
    configured = os.environ.get("AGENTHUB_CLI")
    if configured:
        return [configured]
    fallback = Path("/tmp/agenthub")
    if fallback.exists():
        return [str(fallback)]
    return ["go", "run", "./cmd/agenthub"]


def export_bundle_with_cli(source_dir: str, fmt: str = "json") -> tuple[Path, dict | None]:
    suffix = ".ahubz" if fmt == "archive" else ".ahub"
    target = Path(tempfile.mkdtemp(prefix="agenthub-sync-export-")) / f"bundle{suffix}"
    cmd = _agenthub_cli_command() + ["sync", "export", "--source", source_dir, "--format", fmt, "-o", str(target)]
    subprocess.run(cmd, cwd=ROOT, check=True)
    if fmt == "archive":
        with zipfile.ZipFile(target) as zf:
            manifest = json.loads(zf.read("manifest.json").decode("utf-8"))
        return target, manifest
    return target, json.loads(target.read_text("utf-8"))
