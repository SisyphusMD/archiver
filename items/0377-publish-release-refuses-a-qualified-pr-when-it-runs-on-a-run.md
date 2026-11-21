# #377 Publish Release refuses a qualified PR when it runs on a runner with a different server URL

- Kind: issue, state: open, by cody, created 2026-11-20T18:32:58-08:00
- Labels: none; milestone: none

Publish Release run 41557 (v0.11.9, PR #376) failed with "This PR head did not pass Prepare Release" although 402a470 carried a success `Release / Qualified candidate` status from run 858.

Cause: `publish-release.yml` matches the status `target_url` against `$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/`. Prepare Release ran on a pool runner registered with `http://forgejo.forgejo.svc.cluster.local`, so the status URL uses that host; the first Publish attempt landed on a runner whose server URL is `https://forgejo.bryantserver.com` (its checkout fetched from there), so the prefix did not match. A rerun on a pool runner passed and tagged v0.11.9.

Fix: match on the repository path and run number, independent of the host the runner was registered with (or check the status creator instead of the URL). Same check exists in `release.yml` for an existing release PR; review both. Also find which runner registered with the public URL.

## cody, 2026-11-21T00:42:58-08:00

Status: open, no PR yet. Only publish-release.yml:43-50 matches the URL (release.yml:189 writes it). Next: make the match host-independent (repo path + run number, or the status creator), test-first, with a cx review since it is a release trust check; also identify the runner registered with https://forgejo.bryantserver.com. No crons or holds.
