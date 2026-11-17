# #153 chore: delete the bash implementation (ADR 10 step 4)

- Kind: pull, state: closed, by cody, created 2026-10-05T17:46:48-07:00
- Labels: none; milestone: none
- refs/pull/153/head -> main, merged: True

ADR 10 step 4, part 2b: the bash implementation is deleted (6,145 lines). Nothing reached it since #152.

- Gone: `archiver.sh`, `lib/core`, `lib/features`, `lib/scripts` (the logo in `lib/logos` stays), the bats tests of the bash library, and the kit/envelope parity tests (nothing left to compare). `next_version.bats` (release logic) stays; shellcheck now checks `.forgejo/scripts/next-version.sh`.
- `restore-caps.sh` and `prune-in-use.sh` sourced bash functions; they now drive `archiver auto-restore` (a held restore keeps its revision through a prune via Go's registration).
- Dead Go paths for bash-run backups removed: runctl's outside stop, `runlock.HeldByGo`. Comments that pointed at bash files now state their own reasons.
- `docs/examples` (copied to `/opt/archiver/examples`): `pre-backup`, `post-backup`, `filters`, `post-restore` replace the legacy settings and restore-service examples.

Never-break coverage before deletion: #150 (cross-version incl. HOSTNAME override and sanitized names).
Local: Go unit tests, bats, all 33 integration tests as the workflow runs them, full e2e suite.
