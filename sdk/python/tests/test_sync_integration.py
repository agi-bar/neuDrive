from __future__ import annotations

import importlib.util
import json
import os
import sys
import time
import unittest
from pathlib import Path

import httpx

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "sdk" / "python"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

from agenthub import AgentHub  # noqa: E402
from sync_fixture import materialize_source  # noqa: E402

BASE_URL = os.environ.get("AGENTHUB_TEST_URL", "").rstrip("/")


def _load_tool_module():
    spec = importlib.util.spec_from_file_location("ahub_sync_tool", ROOT / "tools" / "ahub-sync.py")
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load ahub-sync tool module")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _register_user() -> tuple[str, str]:
    slug = f"py-sync-{int(time.time() * 1000)}"
    email = f"{slug}@test.local"
    password = "agenthub-sync-1234"
    response = httpx.post(
        f"{BASE_URL}/api/auth/register",
        json={"slug": slug, "email": email, "password": password},
        timeout=30.0,
    )
    response.raise_for_status()
    body = response.json()
    return body["access_token"], slug


def _create_sync_scoped_token(jwt_token: str) -> str:
    response = httpx.post(
        f"{BASE_URL}/api/tokens",
        headers={"Authorization": f"Bearer {jwt_token}"},
        json={
            "name": "python-sync-test",
            "scopes": ["read:bundle", "write:bundle"],
            "max_trust_level": 3,
            "expires_in_days": 1,
        },
        timeout=30.0,
    )
    response.raise_for_status()
    return response.json()["token"]


@unittest.skipIf(not BASE_URL, "AGENTHUB_TEST_URL not set")
class TestPythonSyncIntegration(unittest.TestCase):
    def setUp(self) -> None:
        jwt_token, _ = _register_user()
        self.token = _create_sync_scoped_token(jwt_token)
        self.tool = _load_tool_module()

    def test_json_bundle_preview_import_export_and_history(self) -> None:
        source_dir = materialize_source(multiplier=1)
        bundle = self.tool.build_bundle(source_dir, "merge")

        with AgentHub(BASE_URL, self.token) as hub:
            preview = hub.preview_bundle(bundle=bundle)
            self.assertTrue(preview.get("fingerprint"))

            result = hub.import_bundle(bundle)
            self.assertGreater(result.get("files_written", 0), 0)

            exported = hub.export_bundle("json")
            self.assertIn("skills", exported)

            jobs = hub.list_sync_jobs()
            self.assertGreaterEqual(len(jobs), 2)

    def test_archive_session_resume_commit(self) -> None:
        source_dir = materialize_source(multiplier=3)
        bundle = self.tool.build_bundle(source_dir, "merge")
        archive, manifest = self.tool.build_archive(bundle, {
            "include_domains": [],
            "include_skills": [],
            "exclude_skills": [],
        })

        with AgentHub(BASE_URL, self.token) as hub:
            session = hub.start_sync_session({
                "transport_version": "ahub.sync/v1",
                "format": "archive",
                "mode": "merge",
                "manifest": manifest,
                "archive_size_bytes": len(archive),
                "archive_sha256": manifest["archive_sha256"],
            })
            first_end = min(session.chunk_size_bytes, len(archive))
            state = hub.upload_part(session.session_id, 0, archive[:first_end])
            self.assertIn(state.status, {"uploading", "ready"})

            resumed = hub.resume_session(session.session_id, archive)
            self.assertIn(resumed.status, {"ready", "uploading"})

            preview = hub.preview_bundle(manifest=manifest)
            result = hub.commit_session(session.session_id, preview.get("fingerprint"))
            self.assertGreater(result.get("files_written", 0), 0)

            job = hub.get_sync_job(session.job_id)
            self.assertEqual(job.status, "succeeded")


if __name__ == "__main__":
    unittest.main()
