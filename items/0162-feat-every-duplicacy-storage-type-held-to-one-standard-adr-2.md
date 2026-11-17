# #162 feat: every Duplicacy storage type, held to one standard (ADR 23, 25)

- Kind: pull, state: closed, by cody, created 2026-10-06T10:36:09-07:00
- Labels: none; milestone: none
- refs/pull/162/head -> main, merged: True

Stacked after #160 (its two commits show here until #160 merges).

- **Every Duplicacy 3.2.5 storage type** is a configuration type: `sftpc`, `b2-custom`, `s3c`, `minio`, `minios`, `wasabi`, `azure`, `gcs`, `gcd`, `one`, `odb`, `dropbox`, `swift`, `webdav`, `webdav-http`, `smb`, `storj`, `fabric`, beside `local`, `sftp`, `b2`, `s3`. One table (`internal/config/types.go`) drives settings, file-only secrets, validation, `init`, the Duplicacy URL and credential variables, the recovery kit's rclone remote, the envelope and check intervals. `local`/`sftp`/`b2`/`s3` keep exactly their variables, URLs and credential names (pinned by tests).
- **Google Drive, OneDrive, Dropbox use your own app**, never duplicacy.com. OneDrive's rotating token: Duplicacy, rclone and the recovery kit use a refreshed copy; the envelope says to sign in instead.
- **Envelope:** every block carries a command that downloads the kit from any machine (`sftp`, or a self-contained `rclone copyto`). B2/S3 pages gain it, so older printed envelopes show OUT OF DATE once.
- **A storage Duplicacy cannot open is explained** (missing directory: create it first; or unreachable, with the reason) in backups, copy workers and restores.
- **Session summary** no longer says "completed successfully" for a run with errors.
- **Tests:** `tests/integration/backends.sh` (new CI step) runs every emulated type as primary (backup, kit fetched independently, the envelope's printed command run as printed, restore) and as a copy (inline copy, maintenance check/prune, copy worker hand-off, check, mirroring, exhaustive prune, pause/resume/stop, restore from it), against Garage, SeaweedFS (V2 signing), atmoz/sftp, rclone WebDAV, Samba, Swift SAIO, Azurite, fake-gcs, plus a Dropbox token-wiring stand-in. Wasabi's mirroring/exhaustive prune need its own MOVE request and run only nightly.
- **Nightly (ADR 25):** `nightly-backends.yml` + `real-backends.sh` run the same two phases against real services once the `NIGHTLY_*` secrets exist (test accounts still to be created); without them it skips.

Verified locally: all integration scripts, every backends case, full e2e (891 s), unit tests.
