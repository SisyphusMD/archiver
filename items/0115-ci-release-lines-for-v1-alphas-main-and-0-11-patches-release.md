# #115 ci: release lines for v1 alphas (main) and 0.11 patches (release/0.11)

- Kind: pull, state: closed, by cody, created 2026-09-29T07:34:13-07:00
- Labels: none; milestone: none
- refs/pull/115/head -> main, merged: True

ADR 9 prerequisite, before main takes its first breaking change.

- `release.yml`: new `alpha` bump; version and branch rules move to `.forgejo/scripts/next-version.sh` (bats: `tests/next_version.bats`, 8 cases). main cuts 0.x until its first 1.0.0 alpha, then alphas and 1.0.0 (and 1.x again once 1.0.0 ships); `release/<major>.<minor>` cuts only that line's patches, counting only tags reachable from HEAD. Alphas skip the compose/README/guide image bump.
- `docker-publish.yml`: prerelease tags skip the version-refs check and are created as prereleases on all three release hosts; no `latest` tag was ever pushed and metadata-action already skips major/minor tags for prereleases. PRs to `release/**` get CI. The bats job runs the pinned image with git added (Renovate-tracked via the inline `# renovate:` convention).

After merge, `release/0.11` must be fast-forwarded to main so its own workflow copy has these paths (possible while main has no breaking change).

Local: bats 98/98 (CI-equivalent run), shellcheck clean, yamllint clean, Codex review clean after one real fix (post-1.0 patches were blocked).
