#!/usr/bin/env python3
"""Verify an Archiver release PR and reproduce its channel-specific release tree."""

import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path

NUMBER = r"(?:0|[1-9][0-9]*)"
SHA = re.compile(r"[0-9a-f]{40}")
CHANNEL = re.compile(rf"main|release/{NUMBER}\.{NUMBER}")
TAG = re.compile(rf"v{NUMBER}\.{NUMBER}\.{NUMBER}(?:-(?:alpha|rc)\.[1-9][0-9]*)?")
PREPARE = Path(__file__).resolve().with_name("prepare-release-tree.sh")


def git(*args, cwd=None):
    return subprocess.check_output(["git", *args], cwd=cwd, text=True).strip()


def reproduce(base, channel, bump, date):
    with tempfile.TemporaryDirectory(prefix="archiver-release-") as directory:
        git("worktree", "add", "--detach", directory, base)
        try:
            version = subprocess.check_output(
                ["bash", PREPARE, channel, bump, date], cwd=directory, text=True,
            ).strip()
            git("add", "-u", cwd=directory)
            return version, git("write-tree", cwd=directory)
        finally:
            git("worktree", "remove", "--force", directory)


def verify(pr, repository, number):
    if pr["number"] != number or pr["flow"] != 0 or pr["state"] != "closed" or pr["merged"] is not True:
        raise ValueError("expected a merged normal release pull request")
    head, base = pr["head"], pr["base"]
    if any(side["repo"]["full_name"] != repository for side in (head, base)):
        raise ValueError("release must originate in this repository")
    channel, branch = base["ref"], head["label"]
    tag = branch.removeprefix("release/")
    if (
        not CHANNEL.fullmatch(channel) or not TAG.fullmatch(tag) or branch != f"release/{tag}"
        or head["ref"] not in (branch, f"refs/pull/{number}/head")
    ):
        raise ValueError("invalid release channel or release branch")

    candidate, merge = head["sha"], pr["merge_commit_sha"]
    if not all(isinstance(sha, str) and SHA.fullmatch(sha) for sha in (candidate, merge)):
        raise ValueError("expected full candidate and merge SHAs")
    git("fetch", "--tags", "origin", f"refs/heads/{channel}")
    if merge not in git("rev-list", "--first-parent", "FETCH_HEAD").splitlines():
        raise ValueError("release merge is absent from the primary's channel history")
    object_exists = subprocess.run(
        ["git", "cat-file", "-e", f"{candidate}^{{commit}}"], capture_output=True, check=False,
    )
    if object_exists.returncode:
        git("fetch", "--no-tags", "origin", f"refs/pull/{number}/head")
        if git("rev-parse", "FETCH_HEAD") != candidate:
            raise ValueError("the primary PR ref no longer points at the qualified candidate")
    tree = git("rev-parse", f"{merge}^{{tree}}")
    if tree != git("rev-parse", f"{candidate}^{{tree}}"):
        raise ValueError("merged tree differs from the prepared release")
    parent = git("rev-parse", f"{merge}^")
    if git("rev-list", "--parents", "-1", candidate).split() != [candidate, parent]:
        raise ValueError("the candidate is stale or is not one prepared commit")

    version = tag[1:]
    changelog = git("show", f"{merge}:CHANGELOG.md")
    heading = next((line for line in changelog.splitlines()
                    if line.startswith("## [") and not line.startswith("## [Unreleased]")), "")
    date = re.fullmatch(rf"## \[{re.escape(version)}\] - ([0-9]{{4}}-[0-9]{{2}}-[0-9]{{2}}).*", heading)
    subject = git("show", "-s", "--format=%s", candidate)
    intent = re.fullmatch(rf"release: {re.escape(version)} \((patch|minor|major|alpha|rc) on {re.escape(channel)}\)", subject)
    if date is None or intent is None:
        raise ValueError("the changelog and prepared version do not match the release branch")

    ref = f"refs/tags/{tag}"
    remote = dict(line.split("\t")[::-1] for line in git(
        "ls-remote", "origin", ref, ref + "^{}",
    ).splitlines())
    if remote:
        if remote.get(ref + "^{}") != merge or git("cat-file", "-t", remote[ref]) != "tag":
            raise ValueError("existing tag does not identify this release")
    else:
        if reproduce(parent, channel, intent[1], date[1]) != (version, tree):
            raise ValueError("the qualified release no longer matches the current channel policy and tree")

    return {"channel": channel, "tag": tag, "commit": merge, "candidate": candidate}


def main():
    if len(sys.argv) != 3 or not re.fullmatch(r"[1-9][0-9]*", sys.argv[2]):
        sys.exit("usage: verify-release-pr.py OWNER/REPO PR_NUMBER")
    try:
        result = verify(json.load(sys.stdin), sys.argv[1], int(sys.argv[2]))
    except (ValueError, KeyError, TypeError, AttributeError, subprocess.CalledProcessError) as exc:
        sys.exit(f"Cannot verify release PR: {exc}")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
