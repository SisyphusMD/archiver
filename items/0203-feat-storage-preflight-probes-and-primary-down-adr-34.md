# #203 feat: storage preflight probes and PRIMARY DOWN (ADR 34)

- Kind: pull, state: closed, by cody, created 2026-10-09T17:43:37-07:00
- Labels: none; milestone: none
- refs/pull/203/head -> main, merged: True

ADR 34.

- kit.Probe: rclone stat of the storage config (2 min overall deadline incl. remote setup, 1 low-level retry, follows a linked local config). Not created yet is reachable, except a missing directory on sftp/sftpc/webdav/smb (Duplicacy cannot create it). Once the primary holds backups (backup-state records its URL) or existed at run start, a missing config/directory is the storage gone (an unmounted volume) -> never re-created on the empty mountpoint.
- Backup: primary probed first; down -> one PRIMARY DOWN (new notify.Critical: Pushover priority 1, ntfy 5, Apprise failure), every service skipped with no hooks run, health FAILING. Lost mid-run: a failed service re-probes after its post hook; no further service starts (also re-checked after a pause wait); copies and kit skipped. Re-probed before copies and the copy retry. Inline secondaries probed at start and after a failed registration; unreachable ones skipped with one line (and from the kit).
- Copy workers probe primary and target before each pass, upkeep, exhaustive prune and check.
- Maintenance probes the primary before preparing (down fails the run: secondaries register through it), each secondary before registration, every storage before prune; a stop during a probe ends the run.
- sftpc rclone remotes offer Duplicacy's compatibility ciphers/kex (also fixes kit uploads to sftpc).
- tests/integration/primary-down.sh; backends.sh diagnose expectations now PRIMARY DOWN lines. 49 integration scripts + e2e pass locally. 18 Codex rounds, last clean.
