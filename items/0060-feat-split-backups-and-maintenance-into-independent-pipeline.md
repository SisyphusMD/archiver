# #60 feat: split backups and maintenance into independent pipelines

- Kind: pull, state: closed, by cody, created 2026-07-12T11:40:55-07:00
- Labels: none; milestone: none
- refs/pull/60/head -> main, merged: True

Splits the single nightly run into two independent pipelines so storage check/prune no longer inflate every backup, and hardens the concurrency model. Intended release: 0.10.0 (minor; contains breaking changes).

## Highlights
- Backup pipeline is hooks -> backup -> parallel copies only; check + prune move to a separate `archiver maintenance` pipeline with its own schedule (`MAINTENANCE_SCHEDULE`), lock, and log (`maintenance.log`).
- Secondary copies run in parallel (overlap bandwidth-free enumeration with upload on upload-capped links); each leg reaped as it finishes.
- Per-storage locks serialize a copy against a check/prune on the same storage; the copy also holds the primary source lock for its duration. Atomic reap-by-rename + hardlink-create primitives; stop-aware waits.
- `PRUNE_EXHAUSTIVE_FREQUENCY` (off/daily/weekly/monthly, default monthly) runs the expensive `-exhaustive` GC on its own per-storage interval, with belated firing on sparse schedules.

## Breaking
- `CRON_SCHEDULE` -> `BACKUP_SCHEDULE` (container fails fast if the old name is set); new `MAINTENANCE_SCHEDULE` (unset = manual maintenance only).
- `ROTATE_BACKUPS` -> `PRUNE_BACKUPS` (old name still translated for bundle configs), plus `CHECK_BACKUPS`.
- `archiver start`/`restart` and `backup prune|retain` removed (they error with guidance). New surface: `archiver backup [--detach]`, `archiver maintenance [exhaustive]`, `archiver stop [backup|maintenance|all] [--immediate]`.

## Validation
- 87 bats, shellcheck parity, and all integration suites (including the new maintenance suite) pass locally.
- Multi-agent audit of the full release: 73 findings verified, fixes applied and re-validated.

## Rollout note
Deployments still on `CRON_SCHEDULE` fail fast on this image, so the nas/vps env rename (+ `MAINTENANCE_SCHEDULE`, + vps `drill.nix`) must ride the same PR as the image bump. Version bump is left to the release workflow.
