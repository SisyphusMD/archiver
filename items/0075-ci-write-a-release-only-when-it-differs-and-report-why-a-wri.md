# #75 ci: write a release only when it differs, and report why a write failed

- Kind: pull, state: closed, by cody, created 2026-08-16T19:26:48-07:00
- Labels: none; milestone: none
- refs/pull/75/head -> main, merged: True

The v0.10.5 tag run went red on **Workflow summary** with `github_failures: 15`. Build-and-push and reconcile both succeeded and the new release was created on all three registries; what failed was a contiguous band of PATCHes to *old* GitHub releases — `v0.8.4` down to `v0.5.0`, with the 18 newer and 11 older ones fine. The v0.10.4 tag run (4194) failed identically, so this has been red for at least two releases.

The cause is still unknown, and that is the point: `reconcile_release` ran `curl -fsSL -X PATCH ... >/dev/null` and printed a bare `update FAILED`, discarding the status code and GitHub's message. Replaying all 15 PATCHes by hand with the identical payload returned `200` every time, and 30 rapid PATCHes provoked no secondary rate limit, so there is nothing to go on after the fact.

Three changes:

**Write only what differs.** The loop revisits every tag on every run, so each release rewrote all 43 releases on all three registries — ~129 writes per run, growing with the tag list, re-sending content that last changed when the tag was cut. `release_matches` compares `{name, body, draft, prerelease}` and skips the PATCH when they are equal. Verified against live data on all three hosts: the stored bodies are byte-identical to what the workflow generates, so a run now makes ~3 writes instead of ~129. `target_commitish` is deliberately excluded — it cannot change for an existing tag, and a host that echoed a branch name instead of the sha would make every release look stale.

**Report the failure.** `api_write` keeps the status code and the first 400 characters of the response body in `API_WRITE_ERROR`, so the next real failure is diagnosable from the log instead of being lost.

**Retry the transient classes.** Up to three attempts on `429`/`5xx` and on GitHub's secondary-rate-limit `403`, never on a genuine permission `403`. The server's `Retry-After` wins over the local 10s/20s ladder when it asks for longer (bounded at 120s; an HTTP-date value falls back to the ladder), since retrying inside an active rate-limit window can extend it.

Two findings from an independent review are folded in: honouring `Retry-After`, and re-checking by tag after a failed *create* — a POST is not idempotent, so a 5xx returned after the host already committed the release would turn the retry into a 409/422 and report failure for a release that exists and is correct.

Tested against a mock API. Attempt counts: `200` → 1, real `403` → 1, `404` → 1, `503` → 3, secondary-rate `403` → 3. `Retry-After: 45` produced 90s of backoff where the ladder would have given 30s; an HTTP-date `Retry-After` fell back to 30s. A POST that 5xx'd after committing was recovered and reported as `created (confirmed after a retried create)`. Negative tests: a changed body, a changed name, a flipped `prerelease`, and malformed JSON from the host all fall through to a write. Shellcheck clean, YAML parses.

Note this job only runs on tag pushes, so PR CI does not exercise it — the next release is the first real run.
