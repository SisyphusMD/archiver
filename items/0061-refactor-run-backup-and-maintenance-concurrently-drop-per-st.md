# #61 refactor: run backup and maintenance concurrently, drop per-storage locks

- Kind: pull, state: closed, by cody, created 2026-07-12T13:38:52-07:00
- Labels: none; milestone: none
- refs/pull/61/head -> main, merged: True

Runs the backup and maintenance pipelines concurrently by removing the per-storage lock subsystem, relying on Duplicacy's own lock-free design instead.

Motivation: the primary-source lock held for the whole copy phase meant maintenance serialized behind an in-progress copy (on a long night, maintenance fired and then waited hours for the copy to finish). A workflow audit of the Duplicacy **v3.2.5** source (adversarial red-team, line-cited) confirmed the locking was unnecessary for data safety: a non-exclusive `check`/`prune` running alongside a `copy` on the same storage can, at worst, make the copy abort loudly — never corrupt a storage, delete referenced data, or leave a partial secondary. Two structural guarantees: a copy reads a mid-prune fossil via the `.fsl` fallback, and it writes the destination snapshot file **last** (after all chunks upload).

Changes:
- Remove the per-storage lock primitives (`lib/core/lockfile.sh`), `STORAGE_LOCK_PREFIX`, the acquire/wait/release calls in the copy phase and `maintain_storage`, both EXIT-trap lock reaps, the entrypoint boot cleanup of storage locks, and the healthcheck storage-lock reporting.
- The copy phase now retries a failed leg **once** automatically (`run_copy_legs` helper) to absorb the rare, transient contention; a fresh copy re-enumerates and resumes. Only a second failure is reported as an error.
- Net **-253 lines**.

Safety guardrail (unchanged, reconfirmed): the maintenance pipeline never emits `prune -exclusive` (the one flag that would make concurrency unsafe); `-exhaustive` is safe and does not imply it.

Validation: build, 78 bats, CI-parity shellcheck, and 10 integration suites all pass — including a new copy-path scenario asserting a leg that fails once then succeeds is retried to a clean run. No config/API/env change; ships as a patch (0.10.1).
