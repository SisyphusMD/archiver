# #152 feat: Go owns the whole CLI; the bash pipeline goes (ADR 10 step 4, ADR 20)

- Kind: pull, state: closed, by cody, created 2026-10-05T17:24:48-07:00
- Labels: none; milestone: none
- refs/pull/152/head -> main, merged: True

ADR 10 step 4, part 2a: nothing reaches bash any more (the bash files themselves go in 2b).

- `help`, usage and argument errors are Go (`cmd/archiver/usage.go`); `help` exits 0.
- The bash backup pipeline is no longer reachable: `goPipeline`, `ARCHIVER_PIPELINE` and `prune-local` (bash maintenance's prune) are removed.
- ADR 20: a container whose services still hold `service-backup-settings.sh` refuses to start (and `run backup` refuses), naming them; the entrypoint accepts `migrate hooks` as its command, so `docker compose run --rm archiver migrate hooks` converts them, then the container starts (`tests/integration/refuse-legacy.sh`).
- A restored `service-backup-settings.sh` in a configured service directory is always migrated.
- Tests: `hook-failures.sh` converted to executable hooks (non-executable hook instead of an unparsable settings file); `migrate-hooks.sh` now expects the pre-migration refusal; `restore-commands.sh` writes its pre-migration snapshot with duplicacy directly; `service-settings.sh` and `hooks-always-post.sh` (bash pipeline only; their contracts are in the e2e hooks tests and stop-go) deleted.

Local: unit tests; all 35 integration tests run as the workflow runs them; full e2e suite.
