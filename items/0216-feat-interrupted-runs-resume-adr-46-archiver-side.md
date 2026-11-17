# #216 feat: interrupted runs resume (ADR 46, Archiver side)

- Kind: pull, state: closed, by cody, created 2026-10-10T03:43:59-07:00
- Labels: none; milestone: none
- refs/pull/216/head -> main, merged: True

ADR 46 without the Duplicacy patch (that follows as its own PR: it touches how snapshots are written, so it gets its own design review and verification).

**57fd21d SIGINT on stop.** `proc.Spec.Interrupt`: Terminate sends INT and waits up to 30 s before TERM/KILL; set on every duplicacy spec (backup, copy, prune, check). Duplicacy 3.2.5 saves its resume point on INT (a hash-mode backup's incomplete snapshot, a check's verified chunks) and nothing on TERM. `stop --immediate` sends INT to duplicacy children (TERM to the rest). Copy workers terminate in the background, so stopping all workers never waits on one; the entrypoint's shutdown waits for the copy flocks too.

**a042d37 run records + catch-up.** `internal/resume`: logs/.run-{backup,maintenance,drill}.json, written aside + fsynced (and the directory) so a power cut after a pre hook still finds it. Backup stages per service: pending / hooked (pre ran, post not) / done. A backup starts by running each `hooked` service's post-backup hook with `ARCHIVER_BACKUP_RESULT=interrupted` (recorded at once, never twice), then backs up unfinished services first. `ARCHIVER_STATE_DIR` moved to logs/.hook-state/<snapshot id> so that post hook finds what pre left. Any normal end clears the record; a stop for a container shutdown (`archiver stop --shutdown`, the entrypoint's; flag cleared at start) keeps it. The daemon runs interrupted kinds once at start (`daemon.Run` catchUp: before the job's schedule, or alone if unscheduled); manual mode starts them from the entrypoint. A drill deletes the copies an interrupted drill recorded, only real `drill-*` dirs directly in the recorded root, outside service dirs, with nothing mounted at or under them.

Tests: tests/integration/fault-interrupt.sh (new, in CI): kill between hooks -> interrupted post hook with the pre hook's token; unfinished first; daemon catch-up for backup + maintenance; shutdown vs asked-for stop; a stopped first backup logs `Incomplete snapshot saved` and the next `Previous incomplete backup contains`; drill leftovers deleted, a recorded service path refused. sigterm-stop.sh now also proves `docker stop` keeps the run and `docker start` reruns and finishes it. Ran locally: both + fault-kill, hook-failures, lock-concurrency, maintenance, pause-resume, restore-drill, stop-go, stop-nonfinal, stop-skips-prune; unit tests -race; golangci-lint clean.

Codex: 9 rounds. Taken partly: a record that cannot be written before a pre hook warns rather than skipping the service (a missed backup is worse than the double fault). Conflicts expected with #215 in worker.go (its hold() should also terminate in the background).
