# #256 test: a secondary added later is registered by its worker alone and fails no maintenance

- Kind: pull, state: closed, by cody, created 2026-10-17T15:28:55-07:00
- Labels: none; milestone: none
- refs/pull/256/head -> main, merged: True

Regression tests for two faults noted open after the copy-workers merge (#133). Both are fixed on main; this proves it and keeps them fixed. Test-only.

**A newly registered secondary could fail maintenance once.** At 6f9c29f, bash maintenance ran from a service repository that did not know a secondary a copy worker had just registered, so its check and prune of that secondary failed. Go maintenance now registers each secondary in its own repository (`internal/maintenance/repo.go` `addSecondary`, from df250e8) and keeps to local while workers run.

**First-time registration contacted the target from the backup, waiting on it.** 7e9167b (#175) made `addStorages` return early under copy workers, but removed the only tests and added none.

New coverage:
- `copy-workers.sh`: restart the daemon with a new local secondary `fourth` and wait for its worker to catch it up. Then:
  - maintenance must pass;
  - after a backup, the service repository's preferences must name `local` but not `fourth`;
  - the worker must copy the new revision to `fourth`.
- `maintenance.sh`: without a daemon, a never-created secondary is created, checked and pruned by the first maintenance, with no error.

Proof the new block catches the faults:
- On an image built at 7e9167b^ it fails: the backup registered `fourth`.
- On an image built at 6f9c29f it fails: `Storage check failed for fourth`.
- On main efc295f it passes.

Both full scripts pass on the bench.
