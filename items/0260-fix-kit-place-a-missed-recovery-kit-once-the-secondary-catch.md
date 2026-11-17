# #260 fix(kit): place a missed recovery kit once the secondary catches up

- Kind: pull, state: closed, by cody, created 2026-10-19T07:34:42-07:00
- Labels: none; milestone: none
- refs/pull/260/head -> main, merged: True

A kit upload that failed (cowanserver timed out at 04:01 on 2026-10-19) or was skipped because the storage was down waited for the next backup, up to a day, with the Recovery Kit Failed incident open, although the message already promised "placed when it is back".

- copier.Events.CaughtUp fires after every caught-up pass.
- The daemon then runs the kit step when that storage is missing from the kit state: storages whose workers are retrying/down are skipped, it gives way to a running placement (new kit flock, `ARCHIVER_KIT_TRY_LOCK`, exit 4 Busy), runs in its own process group, is bounded to 10 min (inside the backup kit step 30 min deadline), and never starts after a stop.
- Backups and manual `recovery-kit` now take the same lock, so two placements never interleave.
- The kit no longer says up to date on all storage targets when it skipped a down storage.

Tests: unit -race, golangci-lint clean; new host-driven tests/integration/kit-recovers.sh (failed upload with copies working, and storage down at backup time; 7/7 passes); recovery-kit, copy-workers, incidents and fault-network still pass. Codex review findings (lock starvation, process tree, stop race) fixed.
