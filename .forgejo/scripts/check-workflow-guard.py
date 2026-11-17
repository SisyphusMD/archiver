"""Every job a pull request can start carries the trust gate, or is named here with the reason."""

from __future__ import annotations

import sys
from pathlib import Path

import yaml

WORKFLOWS = Path(__file__).resolve().parents[2] / ".forgejo" / "workflows"
# Both clauses: a fork PR fails the first, and an AGit PR (pushed to refs/for/<branch>) reports this
# repo as its head, so only the second turns it away.
GATE = (
    "github.event.pull_request.head.repo.full_name == github.repository",
    "github.event.pull_request.flow == 0",
)
# Recognize the job gate as a whole so wrapping just one clause in an OR can't
# pass the test.
TRUSTED = " && ".join(GATE)
TAG_ONLY = "startsWith(github.ref, 'refs/tags/v')"
TRUSTED_CONDITIONS = {
    TRUSTED,
    f"github.event_name != 'pull_request' || ({TRUSTED})",
    f"github.event_name == 'pull_request' && {TRUSTED}",
}

# Exemptions have an expected condition so a release job cannot lose its tag guard.
EXEMPT_CONDITIONS = {
    ("docker-publish.yml", "shellcheck"): "",  # Reads shell sources as data.
    ("docker-publish.yml", "validate-changelog"): "",  # Reads CHANGELOG.md as data.
    ("docker-publish.yml", "build-and-push"): TAG_ONLY,
    ("docker-publish.yml", "create-release"): TAG_ONLY,
    ("docker-publish.yml", "summary"): f"always() && {TAG_ONLY}",
}


def pull_request_jobs(workflows: Path) -> dict[tuple[str, str], str]:
    """Collect the normalized job conditions for `pull_request` workflows.

    `pull_request_target` workflows run main's copy and read the PR only as data, so they are out
    of scope here (SisyphusMD/project-rules#57).
    """
    found = {}
    for path in sorted(workflows.glob("*.y*ml")):
        document = yaml.safe_load(path.read_text()) or {}
        triggers = document.get("on", document.get(True)) or {}
        if "pull_request" not in (triggers if isinstance(triggers, (dict, list)) else [triggers]):
            continue
        for job, spec in (document.get("jobs") or {}).items():
            condition = " ".join(str((spec or {}).get("if", "")).split())
            condition = condition.removeprefix("${{").removesuffix("}}").strip()
            found[(path.name, job)] = condition
    return found


def main() -> int:
    jobs = pull_request_jobs(WORKFLOWS)
    if ("docker-publish.yml", "dockerfile-downloads") not in jobs:
        print("docker-publish.yml: expected to inspect the dockerfile-downloads pull-request job", file=sys.stderr)
        return 1
    ungated = {job: condition for job, condition in jobs.items() if condition not in TRUSTED_CONDITIONS}
    missing = [job for job, condition in ungated.items() if EXEMPT_CONDITIONS.get(job) != condition]
    stale = sorted(EXEMPT_CONDITIONS.keys() - ungated.keys())
    for workflow, job in missing:
        print(f"{workflow}: {job} must enforce both trust clauses or keep its exempt condition", file=sys.stderr)
    for workflow, job in stale:
        print(f"{workflow}: {job} has an unused trust-gate exemption", file=sys.stderr)
    return int(bool(missing or stale))


if __name__ == "__main__":
    sys.exit(main())
