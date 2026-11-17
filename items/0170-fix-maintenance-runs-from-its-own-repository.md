# #170 fix: maintenance runs from its own repository

- Kind: pull, state: closed, by cody, created 2026-10-07T23:06:35-07:00
- Labels: none; milestone: none
- refs/pull/170/head -> main, merged: True

Maintenance ran check and prune from the first service directory with a repository. Duplicacy keeps a prune's pending fossil collections in that repository's cache, so adding a service that sorts first stranded them: their chunks waited for the next exhaustive prune. (V1 checklist, Phase 4 leftover.)

Now:
- Maintenance uses `logs/.maintenance-repo/`, beside the copy workers' repositories (same logs-mount advice in the README).
- A `storages` identities file (written atomically) drops the cache of a storage removed or repointed since, as the copy workers do by hashing names into their repository paths.
- Each run takes over collections that service repositories hold for the same storage name and URL. They are moved (not copied) and renumbered after the repository's own, so the upgrade from 0.11 keeps them.
- The primary is registered first. Each secondary is registered only at its turn, so one that hangs cannot hold up the primary, and one that cannot be registered fails on its own. Registration ends on `archiver stop` or a signal.
- Maintenance no longer refuses to run before the first backup. It also closes the loose end "a newly registered secondary fails maintenance once", since maintenance now registers storages itself.

Tests:
- e2e `TestRunsFossilsKeptForNewService` was red on main in both cases: a collection made by this image (logs mounted), and one left by the 0.11 baseline and taken over on upgrade.
- e2e `TestRunsMaintenanceGoesOnPastBadSecondary`.
- Unit tests for the collection takeover and the identities file.
- Locally green: all maintenance, prune and cross-version e2e tests.

Review (Codex): accepted findings on worker-owned secondaries, stop-awareness, registering secondaries at their turn and the atomic identities write. Rejected maintaining secondaries when the primary cannot be registered: adding a secondary without `-copy <primary> -bit-identical` could create a storage that is not bit-identical (never-break).
