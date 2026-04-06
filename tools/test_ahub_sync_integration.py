from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import time
import unittest
from pathlib import Path

import httpx

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "sdk" / "python" / "tests"))
sys.path.insert(0, str(ROOT / "sdk" / "python"))

from agenthub import AgentHub  # noqa: E402
from sync_fixture import materialize_source  # noqa: E402

BASE_URL = os.environ.get("AGENTHUB_TEST_URL", "").rstrip("/")


def load_tool_module():
    spec = importlib.util.spec_from_file_location("ahub_sync_tool", ROOT / "tools" / "ahub-sync.py")
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load ahub-sync tool module")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def register_user() -> str:
    slug = f"cli-sync-{int(time.time() * 1000)}"
    email = f"{slug}@test.local"
    password = "agenthub-sync-1234"
    response = httpx.post(
        f"{BASE_URL}/api/auth/register",
        json={"slug": slug, "email": email, "password": password},
        timeout=30.0,
    )
    response.raise_for_status()
    jwt_token = response.json()["access_token"]
    scoped = httpx.post(
        f"{BASE_URL}/api/tokens",
        headers={"Authorization": f"Bearer {jwt_token}"},
        json={
            "name": "cli-sync-test",
            "scopes": ["read:bundle", "write:bundle"],
            "max_trust_level": 3,
            "expires_in_days": 1,
        },
        timeout=30.0,
    )
    scoped.raise_for_status()
    return scoped.json()["token"]


@unittest.skipIf(not BASE_URL, "AGENTHUB_TEST_URL not set")
class TestAhubSyncCLI(unittest.TestCase):
    def test_export_preview_push_pull_history(self) -> None:
        token = register_user()
        tool = load_tool_module()
        source_dir = materialize_source(multiplier=2)
        bundle_path = ROOT / ".tmp-cli-sync.ahub"
        archive_path = ROOT / ".tmp-cli-sync.ahubz"
        pulled_path = ROOT / ".tmp-cli-pull.ahubz"
        try:
            subprocess.run(
                ["python3", str(ROOT / "tools" / "ahub-sync.py"), "export", "--source", source_dir, "-o", str(bundle_path)],
                check=True,
            )
            subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "export",
                    "--source",
                    source_dir,
                    "--format",
                    "archive",
                    "-o",
                    str(archive_path),
                ],
                check=True,
            )
            preview = subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "preview",
                    "--token",
                    token,
                    "--api-base",
                    BASE_URL,
                    "--bundle",
                    str(bundle_path),
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            self.assertIn("fingerprint", preview.stdout)

            push = subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "push",
                    "--token",
                    token,
                    "--api-base",
                    BASE_URL,
                    "--bundle",
                    str(archive_path),
                    "--transport",
                    "archive",
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            self.assertIn("files_written", push.stdout)

            subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "pull",
                    "--token",
                    token,
                    "--api-base",
                    BASE_URL,
                    "--format",
                    "archive",
                    "-o",
                    str(pulled_path),
                ],
                check=True,
            )
            self.assertTrue(pulled_path.exists())

            diff = subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "diff",
                    "--left",
                    str(archive_path),
                    "--right",
                    str(pulled_path),
                    "--format",
                    "json",
                ],
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertEqual(diff.returncode, 0, diff.stderr)
            diff_body = json.loads(diff.stdout)
            self.assertTrue(diff_body["equal"])

            history = subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "history",
                    "--token",
                    token,
                    "--api-base",
                    BASE_URL,
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            jobs = json.loads(history.stdout)
            self.assertGreaterEqual(len(jobs), 2)

            archive_bytes = archive_path.read_bytes()
            _, manifest = tool.parse_archive(archive_bytes)
            with AgentHub(BASE_URL, token) as hub:
                session = hub.start_sync_session({
                    "transport_version": "ahub.sync/v1",
                    "format": "archive",
                    "mode": "merge",
                    "manifest": manifest,
                    "archive_size_bytes": len(archive_bytes),
                    "archive_sha256": manifest["archive_sha256"],
                })
                first_end = min(session.chunk_size_bytes, len(archive_bytes))
                hub.upload_part(session.session_id, 0, archive_bytes[:first_end])

            session_file = ROOT / ".tmp-cli-resume.ahubz.session.json"
            session_file.write_text(json.dumps({
                "api_base": BASE_URL,
                "bundle_path": str(archive_path),
                "session_id": session.session_id,
                "preview_fingerprint": "",
                "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

            resumed = subprocess.run(
                [
                    "python3",
                    str(ROOT / "tools" / "ahub-sync.py"),
                    "resume",
                    "--token",
                    token,
                    "--api-base",
                    BASE_URL,
                    "--bundle",
                    str(archive_path),
                    "--session-file",
                    str(session_file),
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            self.assertIn("files_written", resumed.stdout)
            self.assertFalse(session_file.exists())
        finally:
            bundle_path.unlink(missing_ok=True)
            archive_path.unlink(missing_ok=True)
            pulled_path.unlink(missing_ok=True)
            (ROOT / ".tmp-cli-sync.ahubz.session.json").unlink(missing_ok=True)
            (ROOT / ".tmp-cli-resume.ahubz.session.json").unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
