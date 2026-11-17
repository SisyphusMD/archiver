# #241 feat(kit-drill): rehearse recovery from the envelope alone (ADR 51)

- Kind: pull, state: closed, by cody, created 2026-10-13T13:27:49-07:00
- Labels: none; milestone: none
- refs/pull/241/head -> main, merged: True

KIT_DRILL_SCHEDULE (cron, unset = off) and `archiver kit-drill [STORAGE]`: for each offsite storage, fetch the recovery kit with only the credentials the envelope prints (break-glass, else the backup credential the envelope falls back to; SFTP with the break-glass key; OneDrive with Archiver's own token, status noting the sign-in step is left to a person), decrypt with stock openssl (password off argv) in a private scratch dir, and require every expected entry, the same content as the local kit and current secrets. Due until it passes, KIT_DRILL_TIMEOUT (6h), stop/pause; termination cancels and removes decrypted scratch, stale scratch from a killed drill is removed at start; initialization failures raise a run-level incident. Incidents per storage, archiver_kit_drill_* metrics, status.

Tests: unit tests (credential scope, OneDrive fallback, expected entries, secret comparison, cancellation cleanup, init incidents, metrics); tests/integration/kit-drill.sh (host-driven: Garage stand-in + atmoz/sftp; pass, stale kit, revoked S3 and SFTP credentials, recovery) in CI; sftp-runtime, recovery-kit, invalid-cron, deep-check rerun. Two Codex review rounds; findings fixed.
