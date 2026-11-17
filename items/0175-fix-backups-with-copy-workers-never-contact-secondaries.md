# #175 fix: backups with copy workers never contact secondaries

- Kind: pull, state: closed, by cody, created 2026-10-08T06:36:50-07:00
- Labels: none; milestone: none
- refs/pull/175/head -> main, merged: True

Last V1 loose end ("first-time secondary registration contacts the target synchronously"). The registration existed only so maintenance could run from a service repository. Since #170, maintenance uses its own repository, and copy workers already had theirs. So with workers running, `addStorages` returns early. The carry-over, bounded add (5 min `AddLimit`) and per-service old-preferences bookkeeping are removed (net -146 lines). Without workers, a backup still registers secondaries for its inline copies, unchanged.

Tests, locally green: e2e Copy/Worker/CrossVersion/TestRuns/ExitStop, all 27 round-trip integration scripts, and host-driven `backends.sh` (copy lifecycle for every emulated type), `sftp-runtime` and `s3-garage`.

Review (Codex): one finding rejected. It said old secondary fossil collections in service repositories would no longer be adopted. Under copy workers, maintenance never adopts secondary collections anyway, because it maintains only the primary. If workers are later disabled, backups register secondaries again and adoption works then, since the cache directories are still on disk.
