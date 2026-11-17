# #65 fix: recovery kit inherits its storage ownership/permissions instead of forcing root:600

- Kind: pull, state: closed, by cody, created 2026-07-21T19:53:04-07:00
- Labels: none; milestone: none
- refs/pull/65/head -> main, merged: True

The automatic recovery kit was placed `root:600`, so a non-root backup/mirror user pulling the datastore could not read it (rsync exits 23 on that one file). The kit is already encrypted and lives inside the datastore, so it now inherits that storage's ownership/permissions instead of being a locked-down outlier:

- **local**: stamped to match the storage's own Duplicacy `config` (owner:group + mode)
- **sftp**: inherits the connecting user's umask/ownership (same access model Duplicacy's chunks get over that connection)
- **b2/s3**: unaffected (no POSIX permissions)

Existing deployments self-correct once on the first backup (or `archiver recovery-kit`) run after upgrade: a placement-scheme version is folded into the change-detection fingerprint, forcing a one-time re-stamp even when the kit content is unchanged (no `recovery-kit force` needed).
