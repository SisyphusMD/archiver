# #310 fix(deep-check): resume a check a container shutdown stopped when the container starts

- Kind: pull, state: closed, by cody, created 2026-11-02T21:59:29-08:00
- Labels: none; milestone: none
- refs/pull/310/head -> main, merged: True

On 11/1 the NAS redeployed archiver for rc.4 while the monthly local deep check was 71% through its 441k chunks. The check stopped with its verified chunks saved and its run record kept, but nothing read that record back: ADR 46 catch-up covered backups, maintenance, drills and kit drills, not deep checks. So it stayed failed and due until 12/1, and `ArchiverDeepCheckFailed` would have fired for a month.

- On start, interrupted deep checks run again (daemon and no-schedule modes) and resume from their verified chunks.
- Like maintenance and drills, only a container shutdown (or a crash) keeps the record. A pass, a damage failure, a manual stop, a timeout or giving way to a backup ends it and keeps the documented retry at the next slot. A failed check no longer reruns its full read at every restart.
- The record now carries the configured storage name, so names that `config.Sanitize` changes (`local-backups`) map back to their schedule and command. A record for a storage since removed is dropped instead of retried forever.
- Fixed in passing: in no-schedule mode an interrupted named drill ran `archiver drill:<name>`, which is not a command. Both catch-up paths now share `catchUpArgs`.

Tests: shutdown keeps the record and every other end clears it (`TestProgressAndIncidents`); catch-up names match `daemon.Jobs` and map to runnable commands, sanitized names included; a removed storage drops its record. `go vet ./...` and `go test -race ./cmd/... ./internal/...` pass. The new catch-up test fails with the fix removed.
