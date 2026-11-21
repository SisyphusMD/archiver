#!/usr/bin/env python3
"""Print the Prepare Release run number that qualified a candidate, from its combined status.

The URL host is ignored: a runner may be registered under any server URL, and a status
URL is caller-written anyway. Trust comes from looking the run up in this repository's
API and checking its log, which publish-release.yml does next.
"""

import json
import re
import sys
from urllib.parse import urlsplit

CONTEXT = "Release / Qualified candidate"
REPOSITORY = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")


def qualified_run(combined, repository):
    statuses = combined["statuses"]
    if not isinstance(statuses, list) or combined["total_count"] > len(statuses):
        raise ValueError("the status list is incomplete")
    path = re.compile(rf"/{re.escape(repository)}/actions/runs/([1-9][0-9]*)")
    runs = set()
    for status in statuses:
        if status.get("context") != CONTEXT or status.get("status") != "success":
            continue
        url = urlsplit(status.get("target_url") or "")
        match = path.fullmatch(url.path)
        if (url.scheme in ("http", "https") and url.hostname and "@" not in url.netloc
                and not url.query and not url.fragment and match):
            runs.add(match[1])
    if len(runs) != 1:
        raise ValueError("expected exactly one qualification run")
    return runs.pop()


def main():
    if len(sys.argv) != 2 or not REPOSITORY.fullmatch(sys.argv[1]):
        sys.exit("usage: qualified-run.py OWNER/REPO < combined-status.json")
    try:
        print(qualified_run(json.load(sys.stdin), sys.argv[1]))
    except (ValueError, KeyError, TypeError, AttributeError) as exc:
        sys.exit(f"Cannot find the qualification run: {exc}")


if __name__ == "__main__":
    main()
