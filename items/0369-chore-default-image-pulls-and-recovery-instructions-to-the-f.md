# #369 chore: default image pulls and recovery instructions to the Forgejo registry

- Kind: pull, state: closed, by cody, created 2026-11-20T08:27:19-08:00
- Labels: none; milestone: none
- refs/pull/369/head -> main, merged: True

Closes #364

The installation, recovery, migration and storage docs, the bundle-refusal help and the recovery envelope now name `forgejo.bryantserver.com/sisyphusmd/archiver`. GHCR stays a mirror with identical digests. `Page.Fingerprint` treats the new registry string as the old one, so envelopes that were already printed and confirmed stay valid; changed credentials still invalidate them, and a golden-fingerprint test pins this. Cross-version e2e baselines pull the same digests from Forgejo.

Release tooling fixes, from review:
- `prepare-release-tree.sh` now replaces a whole pinned prerelease tag on stable releases. Before, a stable `1.0.1` would have turned the storage OAuth example `1.0.0-rc.7` into `1.0.1-rc.7` (cx).
- The legacy migration guide pins the floating `0.11` tag, so a stable v1 release no longer rewrites it to a v1 image that cannot convert bundles. This was a pre-existing bug (qx).
- `test_release_promotion` covers both.

Checks: go vet, race tests for cmd/internal, shellcheck and the release-promotion test pass locally.

## forgejo-actions, 2026-11-20T08:28:48-08:00

<!-- ai-review -->
**AI review** of `3374329a91`: Clean, well-scoped PR that switches operator-facing image references to the Forgejo registry, preserves backward-compatible envelope fingerprints with a golden test, fixes the release-tree sed to consume prerelease suffixes on stable bumps, and pins the migration guide to the floating 0.11 tag. Tests cover the new fingerprint path and both stable and prerelease sed behaviour.

No findings.

Disagree? Reply on this PR with your reasoning; the reviewer re-reads every reply.
<!-- ai-review-state W10= -->
