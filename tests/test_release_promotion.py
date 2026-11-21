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


class QualifiedRunTest(unittest.TestCase):
    """The run is trusted through the API lookup and log, so the URL host carries no trust."""

    REPO = "Owner/Archiver"

    def run_script(self, statuses, total=None):
        combined = {"total_count": len(statuses) if total is None else total, "statuses": statuses}
        return subprocess.run(
            ["python3", "-I", SCRIPTS / "qualified-run.py", self.REPO],
            input=json.dumps(combined), text=True, capture_output=True, check=False,
        )

    def status(self, url, context="Release / Qualified candidate", status="success"):
        return {"context": context, "status": status, "target_url": url}

    def test_accepts_the_run_whatever_host_the_runner_registered(self):
        for host in ("http://forgejo.forgejo.svc.cluster.local", "https://forgejo.bryantserver.com"):
            with self.subTest(host=host):
                result = self.run_script([
                    self.status("https://ci.example/Owner/Archiver/runs/1", context="CI / lint"),
                    self.status(f"{host}/Owner/Archiver/actions/runs/858"),
                ])
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.strip(), "858")

    def test_refuses_anything_but_one_successful_qualification_of_this_repository(self):
        good = "https://forgejo.example/Owner/Archiver/actions/runs/858"
        cases = {
            "other repository": [self.status("https://forgejo.example/Owner/Other/actions/runs/858")],
            "job suffix": [self.status(good + "/jobs/0")],
            "query": [self.status(good + "?x=1")],
            "fragment": [self.status(good + "#x")],
            "credentials": [self.status("https://u@forgejo.example/Owner/Archiver/actions/runs/858")],
            "not http": [self.status("file:///Owner/Archiver/actions/runs/858")],
            "zero run": [self.status("https://forgejo.example/Owner/Archiver/actions/runs/0")],
            "leading zero": [self.status("https://forgejo.example/Owner/Archiver/actions/runs/0858")],
            "failed": [self.status(good, status="failure")],
            "pending": [self.status(good, status="pending")],
            "wrong context": [self.status(good, context="Release / Qualified")],
            "missing url": [{"context": "Release / Qualified candidate", "status": "success"}],
            "none": [],
            "two runs": [self.status(good), self.status(good.replace("858", "859"))],
        }
        for name, statuses in cases.items():
            with self.subTest(name):
                result = self.run_script(statuses)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertEqual(result.stdout, "")
        truncated = self.run_script([self.status(good)], total=51)
        self.assertNotEqual(truncated.returncode, 0)

    def verify_step(self):
        lines = (SCRIPTS.parent / "workflows/publish-release.yml").read_text().splitlines()
        start = lines.index("      - name: Verify the exact merged release")
        run = next(i for i in range(start, len(lines)) if lines[i] == "        run: |")
        body = []
        for line in lines[run + 1:]:
            if line.strip() and not line.startswith("          "):
                break
            body.append(line[10:])
        return "\n".join(body) + "\n"

    def test_publish_step_accepts_a_status_written_by_a_runner_on_another_server_url(self):
        candidate, merge = "a" * 40, "b" * 40
        log = f"Qualified release commit: {candidate}\n"
        responses = {
            "/pulls/8": {},
            f"/commits/{candidate}/status?limit=50": {"total_count": 1, "statuses": [self.status(
                "http://forgejo.forgejo.svc.cluster.local/Owner/Archiver/actions/runs/858")]},
            "/actions/runs?page=1&limit=1&run_number=858": {"workflow_runs": [{
                "id": 41, "workflow_id": "release.yml", "status": "success",
                "trigger_event": "workflow_dispatch",
                "event_payload": json.dumps({"ref": "refs/heads/main", "inputs": {"channel": "main"}}),
            }]},
            "/actions/runs/41/jobs": [{"id": 7, "name": "Open the qualified release PR", "status": "success"}],
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            scripts = root / ".forgejo/scripts"
            scripts.mkdir(parents=True)
            (scripts / "qualified-run.py").write_text((SCRIPTS / "qualified-run.py").read_text())
            (scripts / "verify-release-pr.py").write_text("import json\nprint(json.dumps(" + repr(
                {"candidate": candidate, "channel": "main", "tag": "v1.0.0", "commit": merge}) + "))\n")
            api = root / "api"
            for suffix, body in responses.items():
                path = api / suffix.lstrip("/").replace("?", "_")
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(json.dumps(body))
            (api / "actions/jobs/7").mkdir(parents=True)
            (api / "actions/jobs/7/logs").write_text(log)
            bin_dir = root / "bin"
            bin_dir.mkdir()
            curl = bin_dir / "curl"
            # The stub serves fixtures for this repository's API only, so a request to any other
            # host or repository fails the step like an unreachable server would.
            curl.write_text(
                "#!/bin/bash\nprefix='https://forgejo.bryantserver.com/api/v1/repos/Owner/Archiver/'\n"
                'url="${@: -1}"; [[ "$url" == "$prefix"* ]] || exit 22\n'
                f'file="{api}/${{url#"$prefix"}}"; file="${{file/\\?/_}}"\n'
                'cat "$file" 2>/dev/null || exit 22\n'
            )
            curl.chmod(0o755)
            output = root / "output"
            env = {
                "PATH": f"{bin_dir}:/usr/bin:/bin", "PR": "8", "TOKEN": "fixture",
                "GITHUB_SERVER_URL": "https://forgejo.bryantserver.com",
                "GITHUB_REPOSITORY": "Owner/Archiver", "GITHUB_OUTPUT": str(output),
            }
            result = subprocess.run(["bash", "-c", self.verify_step()], cwd=root, env=env,
                                    text=True, capture_output=True, check=False)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(output.read_text(), f"tag=v1.0.0\ncommit={merge}\n")
