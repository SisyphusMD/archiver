# #133 feat: copy workers keep the secondary storages caught up

- Kind: pull, state: closed, by cody, created 2026-10-02T07:25:04-07:00
- Labels: none; milestone: none
- refs/pull/133/head -> main, merged: True

ADRs 11, 15, 16 (check intervals, mirroring and target maintenance come next).

- Under the daemon (BACKUP_SCHEDULE set), one copy worker per secondary, each with its own copy repository prepared in its first pass: copies every revision its target lacks, one copy at a time, targets in parallel; retries at 1/5/15 min then every 30, with one retry landing on the 30-minute mark where the target is declared down (one notification, a daily reminder, one on recovery). State persists in logs/.copy-workers.json; status shows it; logs/copies.log.
- A backup hands its copies off over the daemon socket and ends; without a daemon (or with different storage settings than the daemon's) it copies inline as before, under the same per-target copy lock. Forced-bash backups are refused while workers run.
- stop/pause/resume reach the workers (bash scripts call `archiver daemon ctl`).
- Safety: per-storage kernel lock around every init/add (duplicacy 3.2.5 has no create-if-absent), per-target copy lock shared by workers and inline copies, daemon ownership lock, stop generations against races, shutdown cancels passes.
- Backups never contact a secondary in steady state: registrations are carried over from the previous preferences (credentials stripped) while the URL is unchanged; first-time registration is bounded (5 min) and skipped for targets the workers report failing. Recovery-kit step: skips failing secondaries, exit 3 for secondary-only failures (not a handed-off backup's failure), 30 min deadline excluding paused time, own process group.
- Also: fix log rotation for two runs in the same second; fix the lock-release race behind lock-concurrency's intermittent CI failure.

Known residuals: a brand-new secondary is contacted once on first registration; maintenance can fail once for a secondary registered while its worker was still preparing (target maintenance moves into the workers next).
