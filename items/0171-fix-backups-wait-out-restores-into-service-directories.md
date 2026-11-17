# #171 fix: backups wait out restores into service directories

- Kind: pull, state: closed, by cody, created 2026-10-07T23:49:06-07:00
- Labels: none; milestone: none
- refs/pull/171/head -> main, merged: True

A restore already refused to start during a backup, but a backup could start mid-restore and save a half-restored service directory. (V1 checklist loose end; it was there in 0.11 too.)

Now:
- A restore whose destination is, lies in or holds a service directory holds `/var/lock/archiver-restore.lock` until the command ends, its restore hook included.
- A backup probes that lock with its own already held, and the restore checks for a running backup after taking it, so whichever starts second sees the other. A refused backup exits 1 and notifies "Backup Skipped". If the probe errors, the backup refuses rather than assume no restore.
- Overlap is judged against the backup's own expansion of SERVICE_DIRECTORIES, symlinks resolved on both sides, after the destination is created. Pattern fallbacks cover service directories the restore will create below it (wildcard-matched symlinks included). Bracket classes it cannot match exactly count as overlapping, so the guard errs toward keeping backups out.
- Restores elsewhere (a DR test into /tmp) only check that no backup is running, as before.
- duplicacy children inherit the lock, so one that outlives a killed restore still keeps backups out. The restore hook does not inherit it, because a service it starts in the background would hold the lock indefinitely.

Known limit: if archiver is killed (SIGTERM/SIGKILL) while a post-restore hook runs, the hook can outlive the lock. Closing that needs a supervising process; logged as a follow-up, not done here.

Tests:
- e2e `TestRunsBackupRefusedDuringRestore` was red on main (the backup ran during the restore) and is green here.
- Unit tests for the guard (symlinks, missing service directories, wildcard-symlink ancestors, conservative bracket classes) and an updated `TestAutoAll`.
- Locally green: every TestRuns, Restore and CrossVersion e2e test, plus all 27 round-trip integration scripts.

Review (Codex, several passes): accepted path normalization, missing service directories, glob semantics, relative patterns, fail-closed probe, the unconditional backup recheck and keeping the hook from inheriting the lock. Rejected only the supervising-process ask (see Known limit).
