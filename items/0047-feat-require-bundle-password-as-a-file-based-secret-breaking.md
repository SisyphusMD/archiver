# #47 feat: require BUNDLE_PASSWORD as a file-based secret (BREAKING)

- Kind: pull, state: closed, by cody, created 2026-07-07T16:06:51-07:00
- Labels: none; milestone: none
- refs/pull/47/head -> main, merged: True

**BREAKING:** `BUNDLE_PASSWORD` is no longer read from the environment. It is now a file-based secret: read from `BUNDLE_PASSWORD_FILE` if set, otherwise `/run/secrets/bundle_password`. A container that still sets `BUNDLE_PASSWORD` in its environment fails fast at startup with a migration message, rather than silently dropping into env-native mode and failing later with a confusing "no bundle" error.

Rationale: the bundle password decrypts the entire bundle (including the RSA private key), so holding it in an env var leaked it via `docker inspect` and `/proc`. This completes the file-only secrets model from the env-native change.

Docker secrets: `/run/secrets/<name>` is the native Docker/k8s secret mount path, so a Compose or Swarm `secrets:` entry named `bundle_password` is picked up automatically (same `<NAME>_FILE` + `/run/secrets` idiom as the official postgres/mysql images). The compose template and README examples now use `secrets:`.

Migration: write the password to a file, mount it at `/run/secrets/bundle_password`, and remove `BUNDLE_PASSWORD` from `environment:` / `env_file`.

Verified locally: fail-fast on an env `BUNDLE_PASSWORD`; a bundle decrypts from a mounted password file through the real entrypoint and backs up (Docker-secret style); env-native round-trip regression passes. New `tests/integration/bundle-secret.sh` guards the DR path; the CI smoke also asserts the env fail-fast.

No version bump (the release workflow owns that); accumulates under `[Unreleased]` toward the single `0.9.0`.
