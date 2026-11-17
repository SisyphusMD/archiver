# #131 feat: Go backup pipeline and archiver migrate hooks

- Kind: pull, state: closed, by cody, created 2026-10-01T18:03:51-07:00
- Labels: none; milestone: none
- refs/pull/131/head -> main, merged: True

ADR 10 step 2 (pipeline) per ADRs 14 and 20.

- archiver backup runs in Go for env-native deployments with executable hooks (pre-backup, post-backup, filters); same log lines, lock-file format, stages, exit codes and notifications as main.sh, so bash stop/pause/resume/status/healthcheck keep working. A kernel lock (flock) sits over the bash lock format.
- Bundle deployments and services still carrying service-backup-settings.sh stay on the bash pipeline; a partly migrated deployment runs in Go, which refuses unmigrated services loudly; bash refuses services with executable hooks. ARCHIVER_PIPELINE=bash|go overrides.
- archiver migrate hooks writes wrappers that call the existing functions (log_message/log_output/handle_error stand-ins), a filters file, rewrites filter lines naming the settings file, holds the backup lock, rolls back on failure, warns on shared pre/post variables and variable-based or unrewritten filters.
- Recovery kit still bash, run as lib/scripts/recovery-kit-step.sh.
- e2e harness: waits for container readiness (fixes a startup race the faster pipeline exposed) and writes hooks in the image's format via the io.archiver.hooks label.

Verified locally: all 25 in-container + 5 host-driven integration tests, full e2e suite, new migrate-hooks integration test.
