from __future__ import annotations

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

from sync_fixture import materialize_source  # noqa: E402

BASE_URL = os.environ.get("AGENTHUB_TEST_URL", "").rstrip("/")


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
        finally:
            bundle_path.unlink(missing_ok=True)
            archive_path.unlink(missing_ok=True)
            pulled_path.unlink(missing_ok=True)
            (ROOT / ".tmp-cli-sync.ahubz.session.json").unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
