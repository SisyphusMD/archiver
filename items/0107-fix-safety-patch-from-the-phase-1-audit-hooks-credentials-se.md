# #107 fix: safety patch from the Phase 1 audit (hooks, credentials, service dirs, download integrity)

- Kind: pull, state: closed, by cody, created 2026-09-27T11:22:16-07:00
- Labels: none; milestone: none
- refs/pull/107/head -> main, merged: True

The ADR 3 safety patch: fixes from the Phase 1 audit, one commit each. Every fix started with an integration test that failed on the current image.

1. **Hooks**: the post-backup hook always runs once the pre-backup hook has (after a failed backup or a stop too), a failed pre hook skips that service's backup instead of reporting success, a failed post hook or unparseable settings file is an error, and no further service starts after a stop.
2. **Credentials**: `duplicacy set -value` wrote the storage password, RSA passphrase, and B2/S3 keys in plaintext into every service directory's `.duplicacy/preferences` and onto argv. They now go only through `DUPLICACY_<NAME>_*` env vars (RSA passphrase exported; a storage named `default` uses duplicacy's unprefixed names).
3. **SERVICE_DIRECTORIES**: entries matching no directory, paths containing a space (were word-split), and names that cannot form a snapshot ID are reported as errors instead of skipped silently.
4. **Image build**: duplicacy, supercronic, and the docker CLI are verified against pinned per-arch SHA-256s; a CI lint enforces it. Renovate bumps of these three now need their digests updated by hand (the build fails closed until then).
5. **README**: the Kubernetes CronJob example sets a stable pod hostname.

Behavior changes worth noting at release: runs that silently skipped data or ignored a failed pre hook now exit non-zero. The nas and vps hooks all return 0 by design, so they are unaffected.

Verified on a Docker daemon: 28/28 integration tests and 91 bats pass on the head; each commit got a Codex review.
