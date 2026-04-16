from __future__ import annotations

import datetime
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

from neudrive import NeuDrive  # noqa: E402
from sync_fixture import export_bundle_with_cli, materialize_source  # noqa: E402

BASE_URL = os.environ.get("NEUDRIVE_TEST_URL", "").rstrip("/")
DEV_SLUG_CANDIDATES = [
    candidate.strip()
    for candidate in (
        os.environ.get("NEUDRIVE_TEST_DEV_SLUGS") or "demo,de,admin"
    ).split(",")
    if candidate.strip()
]


def _issue_dev_token() -> tuple[str, str]:
    last_error: Exception | None = None
    for slug in DEV_SLUG_CANDIDATES:
        response = httpx.post(
            f"{BASE_URL}/api/auth/token/dev",
            json={"slug": slug},
            timeout=30.0,
        )
        if response.status_code == 404:
            last_error = RuntimeError(f"dev token user {slug!r} not found")
            continue
        response.raise_for_status()
        body = response.json()
        return body["token"], body["user"]["slug"]
    if last_error is not None:
        raise last_error
    raise RuntimeError("no NEUDRIVE_TEST_DEV_SLUGS candidates configured")

def _register_user() -> tuple[str, str]:
    slug = f"py-sync-{int(time.time() * 1000)}"
    email = f"{slug}@test.local"
    password = "neudrive-sync-1234"
    response = httpx.post(
        f"{BASE_URL}/api/auth/register",
        json={"slug": slug, "email": email, "password": password},
        timeout=30.0,
    )
    if response.status_code != 404:
        response.raise_for_status()
        body = response.json()
        return body["access_token"], slug

    # Hosted/postgres test environments do not expose /api/auth/register.
    return _issue_dev_token()


def _create_sync_scoped_token(jwt_token: str) -> str:
    return _create_scoped_token(jwt_token, ["read:bundle", "write:bundle"])


def _create_scoped_token(jwt_token: str, scopes: list[str]) -> str:
    response = httpx.post(
        f"{BASE_URL}/api/tokens",
        headers={"Authorization": f"Bearer {jwt_token}"},
        json={
            "name": "python-sync-test",
            "scopes": scopes,
            "max_trust_level": 3,
            "expires_in_days": 1,
        },
        timeout=30.0,
    )
    response.raise_for_status()
    body = response.json()
    if isinstance(body, dict) and body.get("ok") is True and isinstance(body.get("data"), dict):
        return body["data"]["token"]
    return body["token"]


@unittest.skipIf(not BASE_URL, "NEUDRIVE_TEST_URL not set")
class TestPythonSyncIntegration(unittest.TestCase):
    def setUp(self) -> None:
        jwt_token, _ = _register_user()
        self.jwt_token = jwt_token
        self.token = _create_sync_scoped_token(jwt_token)

    def test_json_bundle_preview_import_export_and_history(self) -> None:
        source_dir = materialize_source(multiplier=1)
        _, bundle = export_bundle_with_cli(source_dir, "json")
        self.assertIsNotNone(bundle)
        bundle = bundle or {}

        with NeuDrive(BASE_URL, self.token) as hub:
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
        archive_path, manifest = export_bundle_with_cli(source_dir, "archive")
        archive = archive_path.read_bytes()
        self.assertIsNotNone(manifest)
        manifest = manifest or {}

        with NeuDrive(BASE_URL, self.token) as hub:
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

            bad_first = bytearray(archive[:first_end])
            bad_first[0] ^= 0xFF
            conflict = httpx.put(
                f"{BASE_URL}/agent/import/session/{session.session_id}/parts/0",
                content=bytes(bad_first),
                headers={
                    "Authorization": f"Bearer {self.token}",
                    "Content-Type": "application/octet-stream",
                },
                timeout=30.0,
            )
            self.assertEqual(conflict.status_code, 409)

            resumed = hub.resume_session(session.session_id, archive)
            self.assertIn(resumed.status, {"ready", "uploading"})

            preview = hub.preview_bundle(manifest=manifest)
            result = hub.commit_session(session.session_id, preview.get("fingerprint"))
            self.assertGreater(result.get("files_written", 0), 0)

            job = hub.get_sync_job(session.job_id)
            self.assertEqual(job.status, "succeeded")

    def test_preview_does_not_write_history_and_scopes_are_enforced(self) -> None:
        source_dir = materialize_source(multiplier=1)
        _, bundle = export_bundle_with_cli(source_dir, "json")
        self.assertIsNotNone(bundle)
        bundle = bundle or {}

        with NeuDrive(BASE_URL, self.token) as hub:
            before_jobs = hub.list_sync_jobs()
            preview = hub.preview_bundle(bundle=bundle)
            self.assertTrue(preview.get("fingerprint"))
            after_jobs = hub.list_sync_jobs()
            self.assertEqual([job.id for job in after_jobs], [job.id for job in before_jobs])

        read_token = _create_scoped_token(self.jwt_token, ["read:bundle"])
        write_token = _create_scoped_token(self.jwt_token, ["write:bundle"])

        with NeuDrive(BASE_URL, read_token) as hub:
            exported = hub.export_bundle("json")
            self.assertEqual(exported.get("version"), "ahub.bundle/v1")
            self.assertGreaterEqual(len(hub.list_sync_jobs()), 1)
            with self.assertRaises(Exception):
                hub.import_bundle(bundle)

        with NeuDrive(BASE_URL, write_token) as hub:
            preview = hub.preview_bundle(bundle=bundle)
            self.assertTrue(preview.get("fingerprint"))
            with self.assertRaises(Exception):
                hub.export_bundle("json")
            with self.assertRaises(Exception):
                hub.list_sync_jobs()

    def test_sync_token_endpoint_clamps_ttl(self) -> None:
        response = httpx.post(
            f"{BASE_URL}/api/tokens/sync",
            headers={"Authorization": f"Bearer {self.jwt_token}"},
            json={"access": "both", "ttl_minutes": 999},
            timeout=30.0,
        )
        response.raise_for_status()
        body = response.json()["data"]
        self.assertEqual(body["scopes"], ["read:bundle", "write:bundle"])
        expires_at = body["expires_at"]
        remaining = (
            datetime.datetime.fromisoformat(expires_at.replace("Z", "+00:00"))
            - datetime.datetime.now(datetime.timezone.utc)
        ).total_seconds() / 60.0
        self.assertLessEqual(remaining, 121)
        self.assertGreater(remaining, 110)

    def test_sync_token_can_introspect_agent_auth_info(self) -> None:
        response = httpx.post(
            f"{BASE_URL}/api/tokens/sync",
            headers={"Authorization": f"Bearer {self.jwt_token}"},
            json={"access": "both", "ttl_minutes": 30},
            timeout=30.0,
        )
        response.raise_for_status()
        body = response.json()["data"]

        with NeuDrive(BASE_URL, body["token"]) as hub:
            info = hub.get_auth_info()

        self.assertEqual(info["api_base"], BASE_URL)
        self.assertEqual(info["auth_mode"], "scoped_token")
        self.assertEqual(info["scopes"], ["read:bundle", "write:bundle"])
        self.assertTrue(info.get("expires_at"))
        self.assertTrue(info.get("user_slug"))


if __name__ == "__main__":
    unittest.main()
