# #378 Publish Release: find the qualification run by repository path, not server URL

- Kind: pull, state: closed, by cody, created 2026-11-21T09:03:50-08:00
- Labels: none; milestone: none
- refs/pull/378/head -> main, merged: True

Closes #377.

Publish Release found the Prepare Release run by requiring the qualification status URL to start with its own runner's `GITHUB_SERVER_URL`. Cluster pool runners use `http://forgejo.forgejo.svc.cluster.local`, while the annex CI runner registers with the cluster at `https://forgejo.bryantserver.com` (bryantserver/annex `nix/services/ci-runner.nix:88`; it is runner `62383437-…`, which took run 41557's first attempt). So a qualified PR was refused whenever Publish landed on the other kind of runner.

`.forgejo/scripts/qualified-run.py` now reads the run number from a URL whose path is exactly `/<repo>/actions/runs/<n>`, on any http(s) host. It keeps every earlier condition: the exact context, the success state, a complete status list, a digits-only run number, the same error message, and exactly one run.

The host was never a source of trust. Anyone who can post a status can write any URL. Trust comes from the next steps, which are unchanged: they look up that run number in this repository's API (`release.yml`, success, dispatched on main for the same channel) and require `Qualified release commit: <candidate>` in its job log.

Tests (`tests/test_release_promotion.py`):
- The helper accepts both runner hosts and refuses other repositories, `/jobs` suffixes, queries, fragments, credentials, non-http URLs, bad run numbers, non-success states, the wrong context, a missing URL, no run, two runs and a truncated list.
- The real Publish verify step runs against a stubbed API, with `GITHUB_SERVER_URL` set to the public URL and the status written with the cluster URL. It fails on main with "This PR head did not pass Prepare Release". With matching hosts the old step passes (the control run), so the failure comes from the defect. It passes with this change.
- Against the live v0.11.9 candidate (402a470), the helper prints 858.

Checks: `python3 -m unittest discover -s tests -p test_release_promotion.py`, actionlint and ruff (E9,F) are clean. Codex reviewed it and found no trust regression; its one finding, the missing workflow-level test, is the step test above.


## forgejo-actions, 2026-11-21T09:34:52-08:00

<!-- ai-review -->
**AI review** of `378c6f836c`: Clean, well-scoped fix for #377: the qualification-run lookup now matches on repository path instead of the runner's server URL, with a thorough test suite including a workflow-level integration test that proves the old code fails and the new code passes.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
