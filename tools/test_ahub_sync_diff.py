from __future__ import annotations

import copy
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "sdk" / "python" / "tests"))

from sync_fixture import materialize_source  # noqa: E402


def load_tool_module():
    spec = importlib.util.spec_from_file_location("ahub_sync_tool", ROOT / "tools" / "ahub-sync.py")
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load ahub-sync tool module")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class TestAhubSyncDiff(unittest.TestCase):
    def setUp(self) -> None:
        self.tool = load_tool_module()
        self.tmpdir = tempfile.TemporaryDirectory(prefix="agenthub-diff-")
        self.workdir = Path(self.tmpdir.name)

    def tearDown(self) -> None:
        self.tmpdir.cleanup()

    def _write_bundle(self, bundle: dict, path: Path) -> None:
        path.write_text(json.dumps(bundle, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    def _write_archive(self, bundle: dict, path: Path) -> None:
        archive, _ = self.tool.build_archive(bundle, {
            "include_domains": [],
            "include_skills": [],
            "exclude_skills": [],
        })
        path.write_bytes(archive)

    def test_diff_handles_json_archive_filters_and_exit_codes(self) -> None:
        source_dir = materialize_source(multiplier=2)
        bundle = self.tool.build_bundle(source_dir, "merge")
        bundle["profile"] = {"preferences": "同步时先 preview"}
        bundle["memory"] = [{
            "title": "diff-memory",
            "source": "fixture",
            "created_at": "2026-04-06T09:30:00Z",
            "expires_at": "2026-04-09T09:30:00Z",
            "content": "first content",
        }]
        bundle["stats"] = self.tool.calculate_bundle_stats(bundle)

        identical_json = self.workdir / "identical.ahub"
        identical_archive = self.workdir / "identical.ahubz"
        self._write_bundle(bundle, identical_json)
        self._write_archive(bundle, identical_archive)

        changed = copy.deepcopy(bundle)
        changed["profile"]["preferences"] = "同步时先 preview，再 push"
        changed["memory"][0]["content"] = "second content"

        skill_name = sorted(changed["skills"])[0]
        changed["skills"][skill_name]["files"]["references/mcp-notes.md"] += "\nchanged text"

        binary_skill_name = next(name for name, skill in changed["skills"].items() if skill["binary_files"])
        binary_path = sorted(changed["skills"][binary_skill_name]["binary_files"])[0]
        blob = changed["skills"][binary_skill_name]["binary_files"][binary_path]
        data = bytearray(self.tool.base64.b64decode(blob["content_base64"]))
        data[0] ^= 0xFF
        blob["content_base64"] = self.tool.base64.b64encode(bytes(data)).decode("ascii")
        blob["size_bytes"] = len(data)
        blob["sha256"] = self.tool.sha256_hex(bytes(data))
        changed["stats"] = self.tool.calculate_bundle_stats(changed)

        changed_archive = self.workdir / "changed.ahubz"
        self._write_archive(changed, changed_archive)

        equal_proc = subprocess.run(
            [
                "python3",
                str(ROOT / "tools" / "ahub-sync.py"),
                "diff",
                "--left",
                str(identical_json),
                "--right",
                str(identical_archive),
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(equal_proc.returncode, 0, equal_proc.stderr)
        self.assertIn("Equal: yes", equal_proc.stdout)

        diff_proc = subprocess.run(
            [
                "python3",
                str(ROOT / "tools" / "ahub-sync.py"),
                "diff",
                "--left",
                str(identical_json),
                "--right",
                str(changed_archive),
                "--format",
                "json",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(diff_proc.returncode, 1, diff_proc.stderr)
        diff = json.loads(diff_proc.stdout)
        self.assertFalse(diff["equal"])
        self.assertGreater(diff["summary"]["profile"]["changed"], 0)
        self.assertGreater(diff["summary"]["memory"]["changed"], 0)
        self.assertGreater(diff["summary"]["files"]["changed"], 0)
        self.assertTrue(any(item["kind"] == "binary" for item in diff["differences"]))

        filtered_proc = subprocess.run(
            [
                "python3",
                str(ROOT / "tools" / "ahub-sync.py"),
                "diff",
                "--left",
                str(identical_json),
                "--right",
                str(changed_archive),
                "--include-domain",
                "skills",
                "--include-skill",
                skill_name,
                "--format",
                "json",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(filtered_proc.returncode, 1, filtered_proc.stderr)
        filtered = json.loads(filtered_proc.stdout)
        self.assertEqual(filtered["summary"]["profile"]["changed"], 0)
        self.assertEqual(filtered["summary"]["memory"]["changed"], 0)
        self.assertGreater(filtered["summary"]["files"]["changed"], 0)

        bad_proc = subprocess.run(
            [
                "python3",
                str(ROOT / "tools" / "ahub-sync.py"),
                "diff",
                "--left",
                str(self.workdir / "missing.ahub"),
                "--right",
                str(identical_archive),
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(bad_proc.returncode, 2)


if __name__ == "__main__":
    unittest.main()
