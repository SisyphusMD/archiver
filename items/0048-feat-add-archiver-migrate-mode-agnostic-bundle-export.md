# #48 feat: add 'archiver migrate' + mode-agnostic 'bundle export'

- Kind: pull, state: closed, by cody, created 2026-07-07T17:37:33-07:00
- Labels: none; milestone: none
- refs/pull/48/head -> main, merged: True

Completes the env-native rework's two-way street.

- **`archiver migrate [OUTPUT_DIR]`** (new) reads the *effective* config and writes env-native adoption materials: `archiver.env` (non-secret `KEY=value`, for a Compose `environment:` block or a k8s ConfigMap) and a `secrets/` dir with one file per secret plus the RSA/SSH keys (Docker secrets / k8s Secret / openbao). The bundle is left untouched.
- **`bundle export` is now mode-agnostic**: it serializes the effective (env-merged) config into the bundle, so a fully env-native deployment can regenerate a portable encrypted bundle for cold restore / DR.
- **`bundle import`**: the SSH keypair is now optional (only sftp needs it); the RSA keypair is what is required. This lets a bundle exported from an env-native deployment (which may have no SSH key) re-import cleanly.

Under the hood: the config var surface (non-secret vs secret regexes) is factored into `config-loader.sh` as a single source of truth, reused by the loader and by the new `config-serialize.sh`, so a serializer can never silently drop a field the loader knows about.

Verified locally: full round-trip `bundle -> migrate -> env-native backup -> mode-agnostic export -> re-import` passes; migrate emits secret-free env + complete secret/key files; all existing integration tests (round-trip, env-native, bundle-secret, scheduler) and bats 36/36 still pass. New `tests/integration/migrate.sh` runs the whole round-trip in CI.

No version bump; accumulates under `[Unreleased]` toward the single `0.9.0`.
