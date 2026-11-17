# #198 Fault injection (ADR 31) and the three defects it found

- Kind: pull, state: closed, by cody, created 2026-10-09T13:31:19-07:00
- Labels: none; milestone: none
- refs/pull/198/head -> main, merged: True

Four fault-injection suites (ADR 31), in CI:

- fault-corruption.sh: chunks only revision 2 has are deleted/altered. Check names the damaged revision, fails and notifies; drill fails and notifies; restoring revision 2 fails; revision 1 restores byte for byte.
- fault-kill.sh: duplicacy, a pre-backup hook, archiver itself (SIGKILL), and a copy worker's copy are killed. Runs fail, post-backup still runs, locks are freed or cleared as stale, no revision added, the next run proceeds and restores; the worker retries and catches up.
- fault-disk.sh (host-driven, tmpfs mounts): full storage, full restore target and full logs volume each fail with a clear error and notification, and recover once space returns.
- fault-network.sh (host-driven, toxiproxy in front of SFTP): a slow link completes; a link cut mid-backup fails with no revision and frees the lock; a link cut mid-copy is retried (duplicacy's own backoff, then the worker) and never reported caught up; both recover and restore.

Defects found and fixed (each its own commit, test-first by the suites above):
1. An unwritable log never notified: every line failed to stdout only. Now notifies once a run.
2. Maintenance logged "Maintenance completed successfully" after a failed check or prune (exit code was right). The summary now counts errors, as backup's does.
3. A failed check did not say which revisions were damaged (duplicacy stops at the first). Checks run with -persist and the error and notification list "damaged revisions: ID revision N, ..." (maintenance and copy workers).

Noted, not changed: a plain check only proves chunks exist; bit rot inside a data chunk is found by drills (or check -chunks, which downloads everything).
