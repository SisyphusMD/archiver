# 46. Interrupted runs resume

- Status: Accepted
- Date: 2026-10-09

## Context

A container restart, host reboot or power cut mid-backup loses the run: nothing is
corrupted (a revision exists only once its snapshot file is written; uploaded chunks are
reused), but the backup waits for its next scheduled time, a service whose pre-backup hook
ran never gets its post-backup cleanup, and Duplicacy re-reads what it had already done.
Duplicacy 3.2.5 saves its own resume point only on SIGINT and only for hash-mode (first)
backups. Freezing the process with CRIU was rejected: it needs privileges beyond the
capability contract, cannot help after a power cut, and cannot restore network connections.

## Options

1. Leave it to the next scheduled run.
2. Resume in Archiver only (catch-up run and hook cleanup on start).
3. Archiver resume plus a Duplicacy patch so it rarely loses its place.

## Decision

Option 3.

## Consequences

- Every stop sends Duplicacy SIGINT, so it saves its resume point before exiting.
- A patch in build/duplicacy saves the resume point every few minutes during any backup
  and uses it for incremental backups too; offered upstream through teaching mode.
- Runs in progress are recorded on the logs volume. On start, Archiver first runs the
  post-backup hook of each service interrupted between its hooks
  (ARCHIVER_BACKUP_RESULT=interrupted), then re-runs the interrupted backup at once,
  unfinished services first; interrupted maintenance and drills run again; leftover
  drill copies are deleted.
- Fault-injection tests (ADR 31) kill the container mid-backup, mid-hook and mid-copy and
  prove the run resumes, uploads nothing twice, and cleans up after its hooks.
