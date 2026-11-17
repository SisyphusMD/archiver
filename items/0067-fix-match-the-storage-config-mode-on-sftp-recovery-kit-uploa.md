# #67 fix: match the storage config mode on sftp recovery-kit uploads

- Kind: pull, state: closed, by cody, created 2026-07-23T18:05:30-07:00
- Labels: none; milestone: none
- refs/pull/67/head -> main, merged: True

0.10.2 made the recovery kit share the datastore's access model, but only on **local** storage targets. On **SFTP** targets the kit was still placed with the connecting user's umask, so where that user's umask is restrictive (e.g. `server` with `077`) the kit landed owner-only and a separate mirror/backup user could not read it — the outlier 0.10.2 set out to remove.

The SFTP upload now reads the mode of the storage's own `config` file and applies it to the kit and its README with a best-effort `chmod` after upload (a server that forbids `SETSTAT` still yields a successful upload), matching what the local path already does. The placement-scheme version is bumped again, so existing deployments re-stamp their already-placed kit once on the first backup (or `archiver recovery-kit`) run after upgrade.

- `symbolic_mode_to_octal` + `recovery_kit_sftp_ref_mode` read `config`'s mode over sftp; unit-tested in `recovery_kit.bats`.
- `tests/integration/sftp-runtime.sh` now asserts the placed kit's mode follows the storage `config` mode.
- Verified locally: bats unit suite, the sftp-runtime integration test, and the local recovery-kit integration test all pass.
