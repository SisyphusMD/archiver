# #215 feat: shell completions, upload limits and copy windows (ADR 44)

- Kind: pull, state: closed, by cody, created 2026-10-10T02:49:39-07:00
- Labels: none; milestone: none
- refs/pull/215/head -> main, merged: True

Stacked on #210 (the generated reference): merge that first.

**`archiver completion bash|zsh|fish`** (caf8237): one command table, held to the usage line by a test; bash completes in a test, zsh works sourced and from `$fpath` on the first autoloaded call (tested where zsh exists), fish checked by hand.

**Upload limits and copy windows** (0640b14):
- `STORAGE_TARGET_N_UPLOAD_LIMIT` (kB/s): `-limit-rate` on backups to the primary, divided among the services backing up at once (so the cap holds; refused below BACKUP_PARALLELISM); `-upload-limit-rate` on copies (worker and inline, whole limit: copies run one per target).
- `STORAGE_TARGET_N_COPY_WINDOW` (`HH:MM-HH:MM`, local, may wrap midnight; refused on target 1): the worker is held outside it (new status `held`, `held_until`, `archiver_copy_held`), a copy or upkeep running at the close is ended like a stop and resumes at the open (duplicacy skips sent chunks), a failure's retry state is kept; status says when it opens; backup health gives a windowed target a day more before "behind". Without the daemon, a held target is not probed, registered, copied or checked in, rechecked before each copy. The kit is not uploaded to a held target (both modes, rechecked before each upload; a target already holding the current kit stays current; `recovery-kit` says which storages wait; an open kit incident stays open). Window boundaries are found minute by minute (DST-safe, unit-tested on the 2026 New York changes).
- tests/integration/bandwidth.sh (in CI): a duplicacy shim records the flags; inline/worker/held/unreachable-held/kit/parallel/refusal cases.

Codex: 12 rounds. Rejected (repeated): adding window/limit to StorageFingerprint, which identifies storages for worker handoff and maintenance ownership; a `docker exec -e` override of policy should not make a backup copy alongside the workers.
