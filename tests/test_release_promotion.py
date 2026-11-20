"""Exercise exact release merging against an isolated Git primary."""

import json
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parents[1] / ".forgejo/scripts"


class ReleasePromotionTest(unittest.TestCase):
    def test_exact_release_and_replay_across_channels(self):
        cases = (
            ("main", "minor", "0.12.0"),
            ("main", "alpha", "1.0.0-alpha.1"),
            ("main", "rc", "1.0.0-rc.1"),
            ("release/0.11", "patch", "0.11.8"),
        )
        for channel, bump, version in cases:
            with self.subTest(channel=channel, bump=bump), tempfile.TemporaryDirectory() as temporary:
                self.check_release(Path(temporary), channel, bump, version)

    def check_release(self, directory, channel, bump, version):
        repo = directory / "repo"
        repo.mkdir()

        def git(*args):
            return subprocess.check_output(
                ["git", *args], cwd=repo, text=True, stderr=subprocess.DEVNULL,
            ).strip()

        primary = directory / "primary.git"
        git("init", "--bare", str(primary))
        git("init", "-b", "main")
        git("config", "user.name", "Release Tests")
        git("config", "user.email", "fixture@example.test")
        git("config", "commit.gpgsign", "false")
        git("config", "tag.gpgsign", "false")
        git("remote", "add", "origin", str(primary))
        (repo / "CHANGELOG.md").write_text("## [Unreleased]\n\n- Update.\n\n## [0.11.7] - 2026-01-01\n")
        for path in ("compose.yaml", "README.md"):
            (repo / path).write_text("forgejo.bryantserver.com/sisyphusmd/archiver:0.11.7\n")
        (repo / "docs").mkdir()
        (repo / "docs/storage.md").write_text(
            "forgejo.bryantserver.com/sisyphusmd/archiver:1.0.0-rc.7\n"
            "forgejo.bryantserver.com/sisyphusmd/archiver:0.11\n"
        )
        git("add", ".")
        git("commit", "-m", "Base")
        git("tag", "v0.11.7")
        if channel != "main":
            git("branch", channel)
            git("commit", "--allow-empty", "-m", "Newer stable on main")
            git("tag", "v1.0.0")
            git("checkout", channel)
        git("push", "origin", "main", "--tags")

        tag = f"v{version}"
        branch = f"release/{tag}"
        git("checkout", "-b", branch)
        actual = subprocess.check_output(
            ["bash", SCRIPTS / "prepare-release-tree.sh", channel, bump, "2026-11-17"],
            cwd=repo, text=True,
        ).strip()
        self.assertEqual(actual, version)
        pinned = "1.0.0-rc.7" if bump in ("alpha", "rc") else version
        self.assertEqual(
            (repo / "docs/storage.md").read_text(),
            f"forgejo.bryantserver.com/sisyphusmd/archiver:{pinned}\n"
            "forgejo.bryantserver.com/sisyphusmd/archiver:0.11\n",
        )
        git("commit", "-am", f"release: {version} ({bump} on {channel})")
        candidate = git("rev-parse", "HEAD")
        git("checkout", channel)
        git("merge", "--no-ff", "--no-edit", branch)
        merged = git("rev-parse", "HEAD")
        git("push", "origin", channel)

        slug = "Owner/Archiver"
        pr = {
            "number": 8, "flow": 0, "state": "closed", "merged": True, "merge_commit_sha": merged,
            "head": {"repo": {"full_name": slug}, "label": branch, "ref": branch, "sha": candidate},
            "base": {"repo": {"full_name": slug}, "ref": channel},
        }

        def verify():
            return subprocess.run(
                ["python3", "-I", SCRIPTS / "verify-release-pr.py", slug, "8"],
                cwd=repo, input=json.dumps(pr), text=True, capture_output=True, check=False,
            )

        first = verify()
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(json.loads(first.stdout)["commit"], merged)
        git("tag", "-a", "-m", "Release test", tag, merged)
        git("push", "origin", f"refs/tags/{tag}")
        pr["head"]["ref"] = "refs/pull/8/head"
        replay = verify()
        self.assertEqual(replay.returncode, 0, replay.stderr)
        pr["head"]["sha"] = merged
        self.assertNotEqual(verify().returncode, 0)
