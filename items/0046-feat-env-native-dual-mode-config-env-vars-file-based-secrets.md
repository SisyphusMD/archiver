# #46 feat: env-native dual-mode config (env vars + file-based secrets layered over the optional bundle)

- Kind: pull, state: closed, by cody, created 2026-07-07T15:27:08-07:00
- Labels: none; milestone: none
- refs/pull/46/head -> main, merged: True

Env-native configuration as a dual-mode alternative to the encrypted bundle. Config resolves in three layers of increasing precedence: the bundle's `config.sh` (optional baseline / cold-restore path), plain env vars for non-secret settings, and files for secrets (`<NAME>_FILE` or `/run/secrets/<name>`; a secret passed as a raw env var is purged). RSA/SSH keys are materialized from mounted files. This lets a deployment migrate off the bundle one value at a time, or run fully env-native (a k8s ConfigMap + Secret) with no bundle at all.

Also in this change:
- `SERVICE_DIRECTORIES` accepts a colon/newline-delimited scalar (fits one env var or a YAML block); the legacy bash-array form is still read.
- `config-loader` fails fast when `STORAGE_PASSWORD` is shorter than 8 characters (a Duplicacy storage-init requirement) instead of surfacing later as an opaque failure.
- Stale "edit config.sh" guidance in error messages reworded to be mode-neutral.

No version bump (the release workflow owns that). Verified locally: bats 36/36, shellcheck clean, changelog lint OK, plus a bundle-mode round-trip regression, an env-native round-trip, and a successful env-native backup through the real entrypoint (snapshots written) - all under `cap_drop: ALL`.
