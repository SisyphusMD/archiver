# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed
- **The bash implementation:** `archiver.sh` and `lib/` (all but the logo) are gone from the image. `docs/examples` (in the image at `/opt/archiver/examples`) now holds `pre-backup`, `post-backup`, `filters` and `post-restore` examples in place of `service-backup-settings.sh.example` and `restore-service.sh.example`.
- **The bash backup pipeline.** Every command runs in Go. A container whose services still hold `service-backup-settings.sh` refuses to start, naming them; convert them with `migrate hooks` as the container's command (`docker compose run --rm archiver migrate hooks`), which the entrypoint now accepts, and start again. `ARCHIVER_PIPELINE` is gone, and a restored `service-backup-settings.sh` in a configured service directory is always migrated. `archiver help` exits 0.
- **Bundles are no longer read.** A container that finds a mounted `bundle.tar.enc`, a `config.sh`, a `bundle_password` secret or `BUNDLE_PASSWORD` in its environment refuses to start and prints the conversion: one `docker run` of the 0.11 image (`run migrate`, from 0.11.4) writes the same configuration as `archiver.env` plus secret files, keeping snapshot IDs, storages and keys. `archiver bundle export`, `bundle import` and `migrate` are removed (`migrate hooks` stays), `init` writes env-native materials only, and the configuration is read from environment variables and secret files alone. See the README's "Upgrading from a bundle".

### Added
- **`archiver recover KIT`** (ADR 29): rebuilds a lost host from the recovery kit and its password: decrypts it (stock openssl, password on fd 3), writes `archiver.env`, owner-only `secrets/` and `RECREATE.txt` to `/opt/archiver/recovered`, places the keys, checks every storage, and restores every service of the kit's host into the directories `SERVICE_DIRECTORIES` places them in (`--yes`, `--hook`, `--out`), ending with how to recreate the deployment under the same hostname.
- **Restore preview and partial restores** (ADR 29): an interactive restore shows what it would add, replace and delete before restoring (skippable with `NO_PREVIEW=1`) and suggests another directory when the snapshot is a configured service's; `auto-restore` takes `DRY_RUN=1` to show that and restore nothing, and `RESTORE_PATHS` (also in the interactive advanced options) restores only the paths named.
- **`archiver doctor`** (ADR 30): one read-only pass over the configuration and secrets (keys decrypt and match, kit password, notifiers; `--notify` sends a test), every storage (reachable, opens with the password; a missing one is reported, never created), the container (capabilities, logs volume, free space, service directories, hooks) and freshness (newest revision per service per storage, copy workers, kit, envelope, last drill). Exit 1 on any failure.
- **Restore drills** (ADR 28): with `RESTORE_DRILL_SCHEDULE` set, Archiver restores each storage's newest revision of the next service in rotation (`RESTORE_DRILL_SERVICES`, `_STORAGES`, `_DIR`, `_EXCLUDE`) into a scratch directory, lets Duplicacy verify every chunk, checks the file count against the revision's listing, and deletes the copy. A failed drill notifies; `status` shows the last drill per service and storage; the healthcheck warns on a failed drill or none passing within twice the schedule's interval. `archiver drill [SERVICE] [STORAGE]` runs one now; `stop`, `pause` and `resume` act on drills.
- **`LOG_FORMAT=json`** prints the container's logs (`docker logs`) as one JSON object per line, with `time`, `level`, `service`, `log` (archiver, maintenance or copies) and `msg`, for log collectors such as Alloy or Loki. The default stays `text`, and the files in the logs volume are unchanged.
- **Apprise and ntfy** beside Pushover (ADR 36): `APPRISE_URL` (a secret, since it may carry basic auth) with optional `APPRISE_TAGS`, and `NTFY_URL` with an optional `ntfy_token` secret. `NOTIFY_ON` (`failures`, `problems`, `everything`) and per-destination overrides choose what each receives. A kind `APPRISE_TAGS` does not name is sent with the tag `all`, so it reaches every URL of the Apprise configuration (an Apprise API key given no tag notifies only its untagged URLs). An HTTP 424 from Apprise (some URL failed) is reported as a failed notification and not retried.
- **Hook tools in the image:** `btrfs-progs`, and Python 3 with the `lmdb` module, so a hook can take a read-only btrfs snapshot right before its service's backup and compact an LMDB index (such as Garage's) from it without starting a helper container. A btrfs snapshot needs the `SYS_ADMIN` capability, an opt-in the README documents; Archiver itself still needs only `DAC_OVERRIDE` (and `CHOWN` and `FOWNER` to restore ownership). Also in 0.11.7.
- Every envelope block has a command that downloads the kit from any machine (`sftp` for SFTP, a self-contained `rclone copyto` line for the other types), and when Duplicacy cannot open a storage the error says whether its directory is missing or it cannot be reached. The B2 and S3 blocks gain the command too, so an envelope printed before this shows `OUT OF DATE` once: print it again.
- **Every Duplicacy storage type** (ADR 23): besides `local`, `sftp`, `b2` and `s3`, a target may be `sftpc`, `b2-custom`, `s3c`, `minio`, `minios`, `wasabi`, `azure`, `gcs`, `gcd` (Google Drive), `one`/`odb` (OneDrive), `dropbox`, `swift`, `webdav`/`webdav-http`, `smb`, `storj` or `fabric`, each with its own `STORAGE_TARGET_N_*` settings and secret files (README "Storage types"). `init` asks for each type's settings, every type receives the recovery kit and has an envelope block, and checks default to weekly on file servers and consumer drives. Google Drive, OneDrive and Dropbox go through your own app, never duplicacy.com. B2 and S3 targets accept an optional `_PATH` inside the bucket. Existing configurations are unchanged.
- `archiver envelope` writes a printable break-glass page (PDF and HTML) with the recovery password, the decrypt command, and for each storage where the recovery kit sits a credential that can read it, as text and QR codes. A storage's optional break-glass credential (`STORAGE_TARGET_N_BREAKGLASS_*` secret files, for example a read-only B2 key) goes on the page instead of the backup credential, which is otherwise marked FULL ACCESS. `archiver envelope confirm` records what was printed; after each recovery-kit run, `archiver status` and healthcheck flag the page when something on it changes or a year has passed, with one notification.
- `RECOVERY_KIT_EXTRA_PATHS` puts more files in the recovery kit (under `extra/`): a runbook, restore helpers, git bundles. A missing path is reported as an error.
- The primary's prune leaves out revisions still in use: Duplicacy's own dry run decides what the retention policy deletes, and any revision a copy worker is copying or a restore is reading waits for the next prune instead of failing that copy or restore. Mirroring likewise keeps a revision a restore is reading from a secondary.
- Copy workers maintain their secondary storages. They mirror the primary's retention (deleting on each secondary the revisions the primary has pruned, with rails: only this deployment's snapshot IDs, never an ID's newest revision, nothing when the primary's listing fails, and a pass that would delete more than half of an ID's revisions is refused with a notification until `archiver mirror --allow-large`), run the exhaustive prune on `PRUNE_EXHAUSTIVE_FREQUENCY`, and check each secondary on its own interval (`STORAGE_TARGET_N_CHECK_INTERVAL`, `CHECK_INTERVAL`, default 1 day, 7 days for SFTP). `archiver mirror --dry-run` shows the next mirror pass. While workers run, maintenance keeps to the primary.
- Copy workers: on a schedule, each secondary storage has a worker that copies every revision it lacks, retries a failed copy after 1, 5 and 15 minutes and then every 30 minutes, and notifies once when the storage has been failing for 30 minutes, daily while it stays down, and once when it recovers. A backup no longer waits for copies, so a slow or unreachable offsite no longer delays the next backup or makes it fail. `archiver status` shows each storage's copy state, and `logs/copies.log` holds the workers' log. Without a schedule, backups still copy for themselves.
- `archiver migrate hooks` converts each service's `service-backup-settings.sh` into executable `pre-backup` and `post-backup` hooks and a `filters` file. The generated hooks call your existing functions, so they keep working unchanged.

### Security
- **Hooks someone other than their owner could change are refused, and Duplicacy's own scripts never run** (ADR 45). Hooks run as root: a `pre-backup`, `post-backup`, `post-restore` or `restore-service.sh` that is writable by group or others, or sits below a directory that is (sticky ones like `/tmp` excepted), now fails its service with an error naming the fix (`chmod go-w`); every duplicacy command runs with `-no-script`, so a `.duplicacy/scripts/pre-backup` planted in a service's repository no longer runs with every storage credential. New `HOOKS_DIR` keeps hooks outside the backed-up data. **Upgrade:** a deployment whose service directories or hooks are group-writable (a `UMASK=002` setup) must `chmod go-w` them, or move the hooks to `HOOKS_DIR`.
- **Raw secret environment variables are removed at start,** not just ignored: tar, openssl, rclone, ssh and every later archiver process no longer inherit a `STORAGE_PASSWORD` (or any other secret) set as a plain variable. The warning still names each one.
- **Storage settings with a control character are refused.** A newline in `SFTP_PATH` (or any other storage setting) reached the sftp batch the kit upload writes, where `!` lines run local commands.
- **Token copies under `logs/.tokens` are written as new 0600 files renamed into place,** so a symlink or readable file planted at the path never receives the token, and a directory others can write is refused.
- **`RECOVERY_KIT_EXTRA_PATHS` copies only regular files and directories:** a link to a device (`/dev/zero`) or a FIFO no longer fills the disk or blocks the kit, and a directory linked into itself is skipped.
- **The envelope's rclone fetch commands pass credentials in `RCLONE_CONFIG_KIT_*` variables** instead of on rclone's command line, where other users of the machine could read them from the process list.
- **A restore warns about links in its destination that lead outside it.** Duplicacy writes a snapshot's files through an existing symlinked directory rather than replacing it, so files can land in another service's directory; each such link is named before the restore starts.
- **Kit placement on local storage no longer follows symlinks.** Setting the kit's owner and mode went through a link, so a user who owns the storage directory could swap one in after the upload and take ownership of the secrets or keys; the kit is now adjusted through a descriptor opened without following links.
- The Pushover API token and user key no longer appear on `curl`'s command line, where any process in the container could read them from `/proc`. They now reach `curl` on stdin, like every other secret.

### Changed
- **The recovery kit's RECREATE.txt lists the services backed up,** so `archiver recover` restores exactly those and reports any it cannot find. The kit changes with it: every deployment uploads its kit once after upgrading, and again when its set of services changes.
- **Routine notifications are opt-in:** with the default `NOTIFY_ON=failures`, "Backup Complete", "Maintenance Complete" and pause, resume and stop notices are no longer sent. Set `NOTIFY_ON=everything` (or `PUSHOVER_NOTIFY_ON=everything`) to keep receiving them.
- With copy workers running, a backup no longer contacts secondary storages at all. It used to register a new or changed secondary in each service's repository (waiting up to 5 minutes on an unreachable one); the workers and maintenance register storages in repositories of their own.
- **Services back up two at a time** by default (`BACKUP_PARALLELISM`, default 2), each with its own hooks, so one slow service no longer holds up the rest. Set `BACKUP_PARALLELISM=1` to keep backing them up one after another, in order, for example when one service's hook depends on another's having finished.
- The recovery kit is placed on every storage through a bundled, checksum-pinned `rclone` (ADR 24), replacing the per-type `curl` and `sftp` uploads. The kit, its name and its permissions on local and SFTP storages are unchanged, so existing kits stay current. The kit is now placed only beside an existing storage (its Duplicacy `config` must be there), so an unmounted path or a mistyped bucket fails instead of receiving a stray kit. SFTP host keys are trusted on first use, as before.
- Duplicacy is built from its pinned 3.2.5 source with one patch (ADR 26) instead of the released binary: a Dropbox storage can refresh its token through your own Dropbox app (Duplicacy settings `dropbox_client_id` / `dropbox_client_secret`, as for OneDrive) rather than duplicacy.com. Nothing else changes: same version, same storages and snapshots.
- The container's entrypoint is now `archiver entrypoint` (Go) under `tini`, which reaps processes that hooks leave behind and passes `docker stop` on. Start-up checks and messages, `init`, `run <command>`, log forwarding to `docker logs`, and the graceful stop on SIGTERM are unchanged; `/usr/local/bin/docker-entrypoint.sh` is gone, so an `entrypoint:` override naming it must drop it.
- `archiver envelope` and `envelope confirm`, and the envelope check after each recovery-kit refresh, now run in Go. The page, its fingerprint and both files are byte-for-byte what they were, so an envelope already confirmed printed stays current.
- `archiver init` now runs in Go, with the same questions and the same env-native materials (the RSA key is generated by `openssl` as before: PKCS#1, AES-256 encrypted with the generated passphrase). The passphrase reaches `openssl` on a file descriptor instead of through `expect`, which is no longer in the image. A yes/no answer is now read as a line.
- The recovery kit (`archiver recovery-kit [force]` and the kit step after each backup) now runs in Go. The kit holds the same files, records the same state, and decrypts with the same `openssl` command; existing kits are recognized as current, so nothing is re-uploaded.
- `archiver restore`, `auto-restore`, `auto-restore-all` and `snapshot-exists` now run in Go, with the same environment variables, prompts and exit codes. The restore hook is now an executable `post-restore` that receives `ARCHIVER_SERVICE`, `ARCHIVER_SERVICE_DIR`, `ARCHIVER_SNAPSHOT_ID`, `ARCHIVER_RESTORE_REVISION` and `ARCHIVER_RESTORE_STORAGE`; a `restore-service.sh` still runs when there is no `post-restore`. Restore hooks no longer receive storage credentials. A failing restore hook now makes `auto-restore` exit 1 instead of the hook's own exit code, which could read as 2 (unreachable) or 3 (busy). A restored `service-backup-settings.sh` in a configured service directory is migrated to executable hooks.
- `archiver maintenance` now runs in Go. Checks, prunes, the exhaustive schedule, the maintenance record, log lines, stop, notifications and exit codes behave as before.
- `archiver backup` now runs in Go. Hooks are executables that receive `ARCHIVER_SERVICE`, `ARCHIVER_SERVICE_DIR`, `ARCHIVER_SNAPSHOT_ID`, `ARCHIVER_STATE_DIR` and, for `post-backup`, `ARCHIVER_BACKUP_RESULT`; filters live in a plain `filters` file. Logs, lock file, stop, pause, resume, status, exit codes and notifications behave as before.
- Scheduled runs are started by `archiver daemon` instead of supercronic, which is no longer in the image. Schedules are read by the same parser, so every `BACKUP_SCHEDULE` and `MAINTENANCE_SCHEDULE` keeps its meaning, a run still never overlaps itself, and a run that overruns its next slot still skips it. A schedule that can never fire (such as `0 0 30 2 *`) now fails container start instead of being accepted. `docker logs` shows `archiver daemon:` lines with each next run time and each run's exit status in place of supercronic's, and `/tmp/archiver.crontab` no longer exists.
- `archiver status`, `archiver healthcheck`, and `archiver logs` now run in Go instead of bash. `status` and `healthcheck` print exactly what they did before, with the same exit codes; `logs` shows the same log without `tail`'s `==> file <==` headers. They only read the state the bash backup and maintenance pipelines keep.
- `archiver` is now a small Go program, the first step of the v1 rewrite. For now it runs the same bash commands as before, so every command, flag, exit code, and output is unchanged; scheduled runs, `docker stop`, and `init` go through it too. `archiver init` also works inside a running container.

### Fixed

- **Maintenance no longer ends with "Maintenance completed successfully" after a failed check or prune;** its summary counts the errors, as the backup's does.
- **A failed storage check names every damaged revision** ("damaged revisions: nas-app revision 12, ...") in the log and the notification, for maintenance and for the copy workers' checks (`check -persist`).
- **An unwritable log (a full logs volume) now notifies,** once a run; before, every line failed silently apart from stdout.
- A restore that initializes a storage (connecting to one a backup or copy is creating at the same moment) now takes the same storage-creation lock they do, so the two can no longer write different storage configurations.
- A backup no longer starts while a restore into a service directory is running (the restore hook included); it is skipped with a notification, since it would have saved the directory half-restored. A restore already refused to start during a backup.
- A notification that fails on a network error, a rate limit or a Pushover server error is retried twice (after 5 and 10 seconds) instead of being lost, and only a refusal such as bad credentials still says to check the Pushover secrets.
- Maintenance runs from a repository of its own in `logs/.maintenance-repo/` instead of the first service directory with a repository, so adding a service that sorts first no longer strands the pending fossil collections of earlier prunes (their chunks waited for the next exhaustive prune). The first run takes over the collections service repositories hold, and maintenance no longer needs a backup to have run first.
- A backup that had errors no longer ends its session summary with "Backup completed successfully" (it gives the error count), and "Primary storage verified" is logged only when the verification passed.
- `archiver pause` now freezes everything a backup runs, including a hook's own programs (a dump, a `docker exec`), and nothing new starts until `archiver resume`. It used to stop only the programs running at that instant, so a pause landing between two steps let the next one run unpaused. `archiver stop` on the Go pipeline lets the backup stop itself, running its post-backup hooks and reporting its real error count; it used to end it from outside during copies and report no errors.
- `archiver restore`, `migrate`, `bundle export`/`import`, `stop`, `pause`, `resume` and `logs` now exit with their command's status. A failure, such as a restore that could not run, used to exit 0.
- Ctrl+C now ends `archiver logs` at once. It used to leave the viewer running when attached with `docker exec -it`.

## [0.11.2] - 2026-09-29

### Fixed
- A backup that is refused because the previous run still holds the lock now sends a notification with the running backup's PID, stage, and start time. A scheduled backup could be skipped this way when offsite copies ran past the next scheduled start, and the only trace was a line in `docker logs`.

## [0.11.1] - 2026-09-29

### Security
- Hooks no longer inherit storage credentials. The storage password, the RSA passphrase, and B2/S3 keys that are exported for Duplicacy used to stay in the environment of every post-backup hook and of every pre-backup hook after the first service, so any program a hook started could read them. They are now withheld from hooks and still reach Duplicacy.

### Fixed
- A restore in a container without the `CHOWN` or `FOWNER` capability now completes, with restored files owned by root, as its warning already said. It used to abort at the first file not owned by root, so a disaster-recovery restore from a minimal container failed.
- A stopped backup now exits non-zero. A stop that landed while a service's hooks were running could end the run with exit 0 because the stop handler was killed by its own signal. Copies and prune were already skipped.
- A stopped maintenance run now exits non-zero instead of 0.
- The recovery kit is now named after the same host as the snapshot IDs, so an inherited `HOSTNAME` (a Kubernetes Job's stable name) wins over the pod's kernel hostname. Each Job run used to leave a kit under a new name. Kits left under old pod names can be deleted.
- Pull skopeo for the publish steps through the NAS image mirror, which keeps every quay.io/skopeo digest it has served. quay garbage-collects a tag's previous digest when it re-pushes the tag, so a direct pin could 404 and fail a publish after its release tag was cut.

### Dependencies

- chore(deps): update quay.io/skopeo/stable:v1.22.3 docker digest to a45bb18
- chore(deps): update quay.io/skopeo/stable:v1.22.3 docker digest to 3f6fe53

## [0.11.0] - 2026-09-27

### Security
- Storage credentials (the storage password, the RSA passphrase, and B2/S3 keys) are no longer written in plaintext to `.duplicacy/preferences` inside every backed-up service directory and every restore destination, and no longer passed on a `duplicacy set` command line that any process could read from `/proc`. They now reach Duplicacy only through its `DUPLICACY_<NAME>_*` environment variables. Service directories are rewritten without them on the next backup. A directory restored by an older version keeps its old `.duplicacy/preferences` until you delete it, so delete it.
- Image builds now verify every downloaded binary (Duplicacy, supercronic, and the docker CLI) against a pinned SHA-256 for each architecture instead of trusting HTTPS alone, so a replaced release asset fails the build.

### Fixed
- The post-backup hook now runs whenever the pre-backup hook ran, including after a failed backup and after a stop (`archiver stop` or `docker stop`) that lands mid-backup. Both cases used to skip it, leaving whatever the pre-backup hook had stopped (a database, a stack) stopped. Once a stop is requested, no further service's pre-backup hook starts.
- A `SERVICE_DIRECTORIES` entry that matches no directory now logs an error and fails the run instead of being dropped silently, and a path containing a space is no longer split into two paths that do not exist. A directory whose name cannot form a Duplicacy snapshot ID (only letters, digits, `_` and `-` are allowed) is reported with a clear error before its hooks run. Runs that used to skip such a directory without a word now exit non-zero until the entry is fixed.
- Hook exit codes now count. A pre-backup function that returns non-zero (a database dump that failed) skips that service's backup for the run, so its newest revision stays the last good one instead of one auto-restore would pick with a broken dump inside; the run reports an error and every other service still backs up. It used to be ignored, and the run reported success. A non-zero post-backup function is reported as an error, and a `service-backup-settings.sh` with a syntax error is an error that skips its service instead of silently backing it up with no hooks. Hooks that deliberately swallow a failure (log a warning and return 0) behave exactly as before.
- The README's Kubernetes CronJob example now sets a stable pod `hostname`. Without it, every Job run backed up under a new snapshot ID.

### Dependencies

- chore(deps): update quay.io/skopeo/stable:v1.22.3 docker digest to 064d2ce
- chore(deps): update quay.io/skopeo/stable:v1.22.3 docker digest to 6cc6b50
- chore(deps): update quay.io/skopeo/stable:v1.22.3 docker digest to ffc4a6b
- chore(deps): update quay.io/skopeo/stable docker tag to v1.22.3
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 2b7d76d
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 0d85390
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 3dba657
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 0b7500a
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 545723e
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 3af38d0
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to b9ca6a5
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to db41084
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to e5d9c4a
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 8d25aab
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 9fb5c29
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 2a0046b
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to b7659e5
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to b1e0c6c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 0f75798
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4fcfc74
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to be235f6
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 11203e8
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 17da3ac
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 64ac45c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 02053f3
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 8870d39

## [0.10.5] - 2026-08-17

### Fixed
- The recovery kit still landed unreadable to a mirror/backup user on an ACL-backed share (e.g. a Synology volume) after 0.10.4: the staged copy did inherit the destination directory's ACL, and the `chmod` that stamps the kit with the storage's own mode then discarded it. On such a share the ACL *is* the access model — from inside the container every file in the store reads as a plain owner-only mode with no ACL visible at all — so the kit was chmod'ed to the mode it already carried, lost the inherited ACL for nothing, and verified clean because its mode still equalled that of the storage's `config`. The kit is now chmod'ed only when that would actually change its mode, so a placement already matching the storage keeps whatever access the directory granted it. The placement-scheme version is bumped, so existing deployments re-place their already-placed kit once on the first backup (or `archiver recovery-kit`) run after upgrade — no `recovery-kit force` needed.

### Dependencies

- chore(deps): update dependency aptible/supercronic to v0.2.49

## [0.10.4] - 2026-08-11

### Fixed
- The recovery kit is now staged beside its destination and renamed over the live kit only after it verifies byte-for-byte, so a failed or truncated copy can never leave a storage target without a kit. The staged file is also a **fresh inode**, so it inherits the destination directory's ACLs — the previous in-place overwrite reused the original inode and its owner-only ACL state indefinitely, which is how a kit stayed unreadable to a mirror/backup user on an ACL-backed share (e.g. a Synology volume) even after 0.10.2 and 0.10.3 matched its POSIX mode from the storage's `config`. A placement that ends up less readable than that `config` is no longer recorded as uploaded: it is reported and re-placed on the next run rather than frozen as up to date, which is what previously let a single bad placement persist silently until someone ran `recovery-kit force`. SFTP uploads now read the mode back after their best-effort `chmod` and are held to the same check. The placement-scheme version is bumped, so existing deployments re-place their already-placed kit once on the first backup (or `archiver recovery-kit`) run after upgrade — no `recovery-kit force` needed.

### Dependencies

- chore(deps): update docker/login-action action to v4.5.2
- chore(deps): update dependency aptible/supercronic to v0.2.48
- chore(deps): update docker/login-action action to v4.5.1
- chore(deps): update bats/bats docker tag to v1.14.0

## [0.10.3] - 2026-07-24

### Fixed
- The recovery-kit permissions change in 0.10.2 only took effect on **local** storage targets; on **SFTP** targets the kit was still placed with the connecting user's umask, so where that user's umask is restrictive (e.g. `server` with `077`) the kit landed owner-only and a separate mirror/backup user still could not read it — the very outlier 0.10.2 set out to eliminate. The SFTP upload now reads the mode of the storage's own `config` file and applies it to the kit and its README with a best-effort `chmod` after upload (a server that forbids `SETSTAT` still yields a successful upload), matching what the local path already does. The placement-scheme version is bumped again, so existing deployments re-stamp their already-placed kit once on the first backup (or `archiver recovery-kit`) run after upgrade — no `recovery-kit force` needed.

## [0.10.2] - 2026-07-22

### Changed
- The automatic recovery kit is no longer written as a locked-down `root:600` file. It is already encrypted and lives inside the datastore, so on local and SFTP storage targets it now takes the same ownership and permissions as that storage's own Duplicacy files (matched from the storage's `config` file on local targets; inherited from the connecting user's umask on SFTP) rather than being a permissions outlier a backup or mirror user cannot read. Object-store targets (B2/S3) are unaffected — they have no POSIX permissions. Existing deployments correct their already-placed kit automatically on the first backup (or `archiver recovery-kit`) run after upgrade: the kit's placement-scheme version is folded into the change-detection fingerprint, so the one-time re-stamp fires even though the kit content is unchanged — no `recovery-kit force` needed.

### Dependencies

- chore(deps): update actions/checkout action to v7.0.1
- chore(deps): update dependency moby/moby to v29.6.2

## [0.10.1] - 2026-07-12

### Changed
- The backup and maintenance pipelines no longer serialize on per-storage locks — they run concurrently, relying on Duplicacy's own lock-free design. Its two-step fossil collection makes a non-exclusive `check`/`prune` safe alongside a `copy` reading or writing the same storage (verified against the Duplicacy v3.2.5 source: a copy reads a mid-prune fossil via a `.fsl` fallback and writes the destination snapshot last, so a concurrent non-exclusive prune can at worst make a copy abort loudly — never corrupt a storage, delete referenced data, or leave a partial secondary). The practical win: maintenance now runs on its own schedule without waiting for an in-progress copy to finish (previously a copy held the primary storage for its whole duration, so maintenance serialized behind it), and a copy never waits on maintenance. A copy leg that loses that rare race is retried once automatically before being reported as an error. The per-storage lock files (`/var/lock/archiver-storage-*`) and their healthcheck reporting are removed. Note: this relies on the maintenance pipeline never using `prune -exclusive` (it uses non-exclusive prune, and `-exhaustive` — which is safe — never implies `-exclusive`).

## [0.10.0] - 2026-07-12

### Breaking
- **Backups and maintenance are now two independent pipelines.** The backup run is hooks → backup → copies, nothing else; storage check and prune moved to a separate maintenance pipeline with its own schedule, lock, and log (`maintenance.log`). Nightly-cost consequence: the multi-hour storage-wide chunk listings that used to run inside every backup (check on every storage plus `prune -exhaustive` on every storage — up to three full sftp/B2 enumerations per storage per night) are gone from the backup path entirely; secondaries are verified by the copy itself (its destination enumeration re-uploads anything missing, healing instead of just reporting). The two pipelines coordinate through per-storage locks, so a check/prune never overlaps a copy on the same storage (the copy-vs-prune concurrency duplicacy does not document as safe is excluded structurally); the waiting side logs who holds the storage. A maintenance overrun can never delay the next backup.
- **`CRON_SCHEDULE` was renamed to `BACKUP_SCHEDULE`**, and the new `MAINTENANCE_SCHEDULE` drives the maintenance pipeline (unset = maintenance only runs via `archiver maintenance`). A container started with `CRON_SCHEDULE` set fails fast with a rename message — silently ignoring it would mean no scheduled backups. The scheduled crontab now invokes the synchronous verbs (`archiver backup` / `archiver maintenance`), so supercronic sees real job durations and adds its own overlap-skip on top of the pipeline locks. A scheduled deployment without `MAINTENANCE_SCHEDULE` gets a prominent startup warning, and healthcheck warns when any storage has gone more than 8 days without a successful check or prune.
- **`ROTATE_BACKUPS` was renamed to `PRUNE_BACKUPS`** (old name still translated silently for old bundle configs, with a startup warning when set as an env var), joined by `CHECK_BACKUPS` (default true) so shared-storage deployments can disable either phase independently, and `PRUNE_EXHAUSTIVE_FREQUENCY` (off/daily/weekly/monthly, default monthly): the expensive `-exhaustive` chunk-listing garbage collection now runs on its own interval per storage instead of on every prune, tracked in a state file and fired belatedly at the next maintenance run if the schedule is sparser than the frequency. `archiver maintenance exhaustive` forces it.
- **`archiver start` and `archiver restart` were removed** (they error with guidance): `archiver backup --detach` is the background form, with the visible already-running refusal. The `prune|retain` arguments to `backup` were removed likewise. `archiver stop` takes a target: `backup`, `maintenance`, or `all` (default).

### Added
- `archiver maintenance [exhaustive]` — the maintenance pipeline verb: per storage, `check -all -fossils -resurrect` (when `CHECK_BACKUPS`) then `prune -all` with the retention policy (when `PRUNE_BACKUPS`), recording per-storage last-success timestamps that `archiver status` displays and healthcheck monitors. Also available to external schedulers as `run maintenance` (synchronous, exit code propagates).
- **Secondary copies now run in parallel.** Each `duplicacy copy` to a secondary is launched concurrently (still direct children, so pause/stop keep working); on an upload-capped link one storage's bandwidth-free destination enumeration overlaps another's upload, so the copy phase approaches max(legs) instead of sum(legs).
- Phase durations in the logs: each service backup, each copy, and each check/prune logs how long it took, so slow nights are diagnosable from the summary instead of by archaeology.

### Deprecated
- Bundle mode remains supported through 0.10.x.

### Fixed
- `archiver backup --detach` (the background form, formerly `archiver start`) now refuses visibly when a backup is already running (non-zero exit and the same "A backup is already running (PID …)" message the synchronous path prints). Previously the dispatcher backgrounded `main.sh` with its output discarded and printed the success line unconditionally, so a refused start looked like a successful one. A stale lock (dead PID) still starts normally.
- The copy phase now holds the primary storage's lock (as source) for its whole duration, not just each destination's lock, so a maintenance prune can no longer fossilize chunks a concurrent copy is still reading from the primary — closing the one copy-vs-prune overlap the per-storage locks had left open.
- The container clears any leftover pipeline lock and stop-flag files at startup, so a lock left by an unclean prior shutdown can neither fake a live holder (which would refuse every scheduled backup after a PID recycle) nor abort the first backup via a stale stop flag. A stale pipeline lock now reports as a healthcheck warning (self-healing, reaped on the next run) rather than flipping the container UNHEALTHY.

### Dependencies

- chore(deps): update docker/login-action action to v4.4.0

## [0.9.2] - 2026-07-11

### Added
- Automatic recovery kit: with the `recovery_password` secret present (at `/run/secrets/recovery_password` or via `RECOVERY_PASSWORD_FILE`), after each backup archiver assembles a kit holding the full effective configuration (every non-secret setting, every secret, and the keys — exactly what `archiver migrate` emits), generated recreation notes (`RECREATE.txt`: hostname, schedule, every container path that needs a mount, required capabilities), and the deployment manifests when mounted read-only at `/opt/archiver/deployment` (a compose bind, a Kubernetes ConfigMap — e.g. a kustomize `configMapGenerator` under Flux/Argo — or a NixOS bind; captured verbatim), encrypts it to that password (AES-256, pbkdf2), and uploads it as a plain file beside the duplicacy data on every storage target (`archiver-recovery-kit-<hostname>.tar.enc` plus a companion README with the decrypt command). `archiver init` now generates the recovery password, includes it in the emitted materials, and displays it once for the user's password manager, and the shipped compose template mounts both the secret and its own `compose.yaml` — so new deployments maintain a kit by default; existing deployments enable it with `openssl rand -base64 24 > secrets/recovery_password` plus a `secrets:` entry (`archiver migrate` prints this hint when unconfigured). Disaster recovery needs only one reachable storage location — even just a provider web UI — plus the single recovery password: `openssl enc -d -aes-256-cbc -pbkdf2 -in <file> | tar -xvf -` on any machine yields `archiver.env` + `secrets/` + `RECREATE.txt` (+ `deployment/`), everything required to recreate the deployment (the recovery password itself is included, so the recreated deployment maintains its kit immediately). The kit is re-uploaded only when its content actually changes or a target is missing it; the fingerprint deliberately ignores runtime state (for example the `prune|retain` rotation override), so an unchanged config is a nightly no-op. `archiver recovery-kit` pushes on demand and `archiver recovery-kit force` re-pushes everywhere. Upload failures are logged and notified but never fail the backup, and failed targets are retried on the next run. The recovery password must be at least 8 characters and differ from `STORAGE_PASSWORD` (it is the only thing protecting the kit at rest, and the kit contains the storage password). Unlike the bundle, the kit is write-only: nothing at runtime ever reads it back, so it is never a boot dependency.
- The shipped compose template now consumes the emitted `archiver.env` directly via `env_file:` instead of inlining values — nothing to transcribe at setup or recovery, and the file holds no secrets so it can be committed to git alongside the compose file (inlining under `environment:` remains equivalent).

### Fixed
- Removed the `/opt/archiver/bundle` entry from the image's `VOLUME` declaration. Docker Compose carries the previous container's mount over for image-declared volume paths when recreating, so converting a bundle-mode deployment to env-native with `up --force-recreate` inherited the old bundle mount and crash-looped on the bundle-without-password fail-fast. Until you run an image with this fix, convert with `docker compose down && docker compose up -d` instead. Env-native containers also no longer accumulate a useless anonymous volume at that path.

### Dependencies

- chore(deps): update actions/checkout action to v7
- chore(deps): update docker/setup-qemu-action action to v4.2.0
- chore(deps): update bats/bats docker tag to v1.13.0
- chore(deps): update dependency moby/moby to v29.6.1
- chore(deps): update docker/setup-buildx-action action to v4.2.0
- chore(deps): update docker/metadata-action action to v6.2.0
- chore(deps): update docker/build-push-action action to v7.3.0

## [0.9.1] - 2026-07-10

### Changed
- `archiver init` now writes its output (the `env-native/` materials and the escrow `bundle.tar.enc`) to a neutral `/opt/archiver/setup` directory — mount that for init (`-v ./archiver-setup:/opt/archiver/setup`). Bundle-mode deployments still mount the bundle at `/opt/archiver/bundle` at run time; only the init output location moved, so the primary setup flow is no longer branded by the transitional bundle path. init also no longer claims the bundle is required to run Archiver (it is the disaster-recovery escrow).
- Image-build robustness: the three binary downloads (duplicacy, docker-cli, supercronic) retry on transient network errors instead of failing the whole build on a single reset, and the docker-cli archive is downloaded to a file before extraction.

## [0.9.0] - 2026-07-10

### Breaking
- **`BUNDLE_PASSWORD` is no longer read from the environment and must be provided as a file** (a Docker or Kubernetes secret). Every existing deployment that passes it via `environment:` or `env_file` must migrate before upgrading: write the password to a file, mount it at `/run/secrets/bundle_password` (a Docker Compose or Swarm `secrets:` entry named `bundle_password` lands there automatically — see the `secrets:` block in the repo's `compose.yaml`), or point `BUNDLE_PASSWORD_FILE` at another path, and remove `BUNDLE_PASSWORD` from the environment. A container that still has it set fails fast at startup with this migration message rather than silently ignoring it (which would otherwise drop a bundle deployment into env-native mode and fail later with a confusing "no bundle" error). Why: the bundle password decrypts the entire bundle, including the RSA private key, and an environment variable leaks through `docker inspect` and `/proc`.

### Added
- Env-native configuration as a dual-mode alternative to the encrypted bundle, so a deployment can be driven entirely by environment variables plus file-based secrets (for example a Kubernetes ConfigMap + Secret) with no bundle at all. Configuration is now resolved in layers of increasing precedence: the decrypted bundle's `config.sh` (optional baseline, still the cold-restore path), plain environment variables for non-secret settings, then files for secrets. Non-secret settings (`SERVICE_DIRECTORIES`, the `STORAGE_TARGET_N_*` non-secret fields, `ROTATE_BACKUPS`, `PRUNE_KEEP`, `DUPLICACY_THREADS`, `NOTIFICATION_SERVICE`) can be set as env vars and override the bundle. Secrets (`STORAGE_PASSWORD`, `RSA_PASSPHRASE`, `PUSHOVER_USER_KEY`, `PUSHOVER_API_TOKEN`, and each target's `B2_ID`/`B2_KEY`/`S3_ID`/`S3_SECRET`) are read only from files, never from a raw env var: each comes from `<NAME>_FILE` if set, otherwise `/run/secrets/<lowercased name>`, and a secret passed as a raw env var is purged, with a logged warning naming the file path to use instead. The RSA keypair (and optional SFTP key) are supplied the same way and materialized into `/opt/archiver/keys` at startup, with mounted key files overriding bundle-extracted keys. This lets you migrate off the bundle one value at a time: set an env var or mount a secret and it shadows the bundle, and once every value is shadowed you can drop the bundle. With no env or secret overrides and a bundle present, behavior is unchanged.
- `SERVICE_DIRECTORIES` accepts a colon-delimited scalar (for example `/srv/*/:/home/user/data/`) in addition to the bundle's bash-array form, so it fits in a single env var or a YAML block. Newlines are also accepted as separators.
- `archiver migrate [OUTPUT_DIR]` converts a bundle-based deployment to env-native materials: it writes the effective non-secret settings to an env file and every secret (plus the keys) to its own file, ready to load as a Docker Compose `environment:` block or a Kubernetes ConfigMap, and as Docker/Kubernetes secrets or openbao entries. The bundle is left untouched.
- `bundle export` is now mode-agnostic: it serializes the effective (env-merged) configuration into the bundle, so a fully env-native deployment can regenerate a portable, self-contained encrypted bundle for cold restore / disaster recovery.
- `archiver init` now also writes ready-to-deploy env-native materials to `env-native/` in the bundle directory (`archiver.env` + `secrets/`, via the same serializer as `archiver migrate`), alongside the encrypted bundle. A new deployment can go straight to env-native without ever hand-transcribing configuration; the emitted files are plaintext secrets, so move them into a secret store and delete the directory. The shipped `compose.yaml` template now leads with the env-native service; bundle mode is the commented alternative.
- Broad new test coverage, all in CI: secondary-storage copy path incl. restore-from-secondary and copy-failure reporting; multi-service glob expansion with per-service failure isolation; lock concurrency (busy refusal, stale-lock recovery); real-entrypoint `docker stop` during a backup; pause/resume/stop-while-paused against the live process tree; stop during a non-final service; per-service `service-backup-settings.sh` (filters, hook order, isolation); healthcheck states; restore extras (revision pinning, `IGNORE_OWNERSHIP`, `restore-service.sh` hook); malformed `CRON_SCHEDULE` fail-fast; real s3 (MinIO sidecar, TLS) and sftp (sshd sidecar) backup+restore runtime round-trips; and new bats suites for `lockfile.sh`, `SERVICE_DIRECTORIES` expansion, config serialization round-trips, and `format_duration`.

### Deprecated
- Deploying from the encrypted bundle is now the transitional path: env-native configuration (environment variables plus file-based secrets) is the primary, first-class configuration source, and a future release will drop bundle support entirely. The bundle remains fully supported throughout 0.9.x, both for deployment and as the `bundle export` cold-restore artifact.

### Changed
- Startup diagnostics for the file-only secret model: a mounted bundle whose password cannot be resolved now fails fast naming the expected password path (instead of a misleading "no bundle" error, or silently starting env-native and never backing up when keys are also mounted); an explicitly set `BUNDLE_PASSWORD_FILE` or `<NAME>_FILE` pointing at a missing file is a hard error instead of a silent fallback; secret files with Windows line endings (CRLF) are handled instead of failing later as a wrong password; and missing B2/S3 credentials name the secret file to provide.
- The bundle password is no longer left in the container's process environment after the bundle import completes, and no longer appears on the openssl command line (visible in `/proc` while it runs) during bundle import/export. The `ENV BUNDLE_PASSWORD=""` stub left in the image (visible in `docker inspect`) is gone.
- `archiver bundle import` now works from a plain `docker exec` (it resolves the default bundle path and the password file itself, matching `bundle export`).
- The interactive `archiver restore` honors `IGNORE_OWNERSHIP=1` like the non-interactive restore paths.
- `archiver migrate` and `archiver init` write their env-native materials with restrictive permissions from the start (umask), and both warn that the output holds plaintext secrets to be moved into a secret store.
- `config-loader` now fails fast with a clear message when `STORAGE_PASSWORD` is shorter than 8 characters (a Duplicacy storage-init requirement), instead of letting it surface later as an opaque "storage initialization failed".
- `bundle import` now treats the SSH keypair as optional (only sftp targets need it); the RSA keypair is what it requires. This lets a bundle exported from an env-native deployment that has no SSH key re-import cleanly.
- Restore now logs a warning (instead of doing it silently) when it would preserve original ownership but the container lacks `CHOWN`/`FOWNER`, so restored files would land owned by root. This makes it safe to run a backup-only deployment with just `DAC_OVERRIDE` and add `CHOWN` + `FOWNER` only when restoring. The restore is never blocked; set `IGNORE_OWNERSHIP=1` to restore without preserving ownership.

### Fixed
- A failed primary backup is now detected and reported. The backgrounded backup pipeline captured the log reader's PID instead of duplicacy's, so a duplicacy failure was invisible: the run logged the backup as completed, sent a success notification, and exited 0. A failed backup now logs an error, is counted in the completion notification, skips that service's copy/wrap-up, and makes `archiver backup` exit non-zero.
- `archiver stop` requested during the final service's backup now actually stops the run. Previously that path swallowed the stop: the run continued into the storage check, `prune -exhaustive`, and copies to secondary storages, then recorded a successful completion. A stop during any service's backup now terminates duplicacy directly (previously it killed the log reader and waited for duplicacy to die of a broken pipe), skips the storage wrap-up, and records "Backup stopped" without counting the stop as an error.
- When no service completes a backup, the storage wrap-up (check/prune) is skipped instead of silently running from whatever repository directory the last failed service left as the working directory.
- Stale-lock recovery now actually re-takes the lock. Previously a run that cleaned up a dead-PID lock file proceeded without holding the lock itself, so `stop`/`status`/`pause` could not see it and a concurrent backup would have been admitted against the same storage.
- A backup refused because one is already running now says so (`A backup is already running (PID ...)`) instead of exiting 1 silently.
- The container's SIGTERM handler (`docker stop`) now waits for the interrupted backup to record its stop and release the lock before exiting, instead of racing PID-namespace teardown against that cleanup (which could SIGKILL it mid-flight and lose the stop record/notification).
- `archiver healthcheck` no longer reports a healthy env-native deployment as UNHEALTHY: the absence of `config.sh` is only an error when there is no RSA key either. It also detected "run finished despite errors" by grepping for a log line that nothing ever emitted, so any transient error in the recent log window flipped the container UNHEALTHY; it now matches the real end-of-run marker.
- Env-native SFTP deployments can now restore: the entrypoint places `/run/secrets/ssh_public_key` (override `SSH_PUBLIC_KEY_FILE`) alongside the private key, and `archiver migrate` exports it. The SFTP restore path requires both key halves; previously only the private key was placed, so backup worked but restore failed.

### Dependencies

- chore(deps): update dependency aptible/supercronic to v0.2.47

## [0.8.12] - 2026-07-07

### Changed
- Replaced the in-container Debian `cron` daemon with [supercronic](https://github.com/aptible/supercronic) for scheduled backups (`CRON_SCHEDULE`). supercronic runs jobs as the container user instead of forking with `setgid`, so the `SETGID` capability is **no longer required** and has been removed from the documented hardened cap set (`compose.yaml`, `README.md`) — the set is now `DAC_OVERRIDE` + `CHOWN` + `FOWNER`. It also passes the container environment straight through (Debian cron scrubs it). The entrypoint validates the schedule with `supercronic -test` (fail-fast on a malformed `CRON_SCHEDULE`) and runs supercronic backgrounded so the SIGTERM graceful-stop path is preserved. Added `tzdata` so a non-UTC `TZ` resolves. A new CI scheduler smoke test asserts a job fires under `cap_drop: ALL` with no `SETGID`. **Action for hardened deployments:** drop `SETGID` from your `cap_add` when you upgrade to this image.
- Consolidated the per-storage-type credential and URL construction — previously duplicated across the primary-backup, add-copy, and restore code paths — into two shared helpers in `config-loader.sh`: `build_storage_url` and `export_duplicacy_storage_secrets`. No behavior change. Adds bats coverage of the `DUPLICACY_<NAME>_*` credential mapping across all four storage types (the surface the 0.8.10/0.8.11 do-spaces incidents traced to), so a single definition is now the source of truth for both backup and restore.

### Fixed
- Restore silently lost original file ownership under the recommended hardened cap set: the example `compose.yaml` / `README.md` did `cap_drop: ALL` and added back only `DAC_OVERRIDE` + `SETGID`, but restore preserves ownership by default (`-ignore-owner` is opt-in) and Duplicacy recreates the original UID/GID via `chown()`, which requires `CAP_CHOWN` (and `CAP_FOWNER` to set mode/timestamps on other-UID files). `DAC_OVERRIDE` bypasses permission checks but not `chown`, so a restore completed with every file owned by root. Added `CHOWN` + `FOWNER` to the documented cap set. (Default Docker caps already include `CHOWN`; this only affected the hardened `cap_drop: ALL` template.)

## [0.8.11] - 2026-06-05

### Fixed
- The 0.8.10 do-spaces restore fix was still broken in `auto-restore`: `sanitize_storage_name` logs a "name was sanitized" WARN via `log_message`, and `auto-restore.sh` redefines `log_message` to also echo to **stdout** — so callers doing `name="$(sanitize_storage_name …)"` captured that log line into the name, re-corrupting `DUPLICACY_<NAME>_*`. The function now redirects its `log_message` to stderr, so only the sanitized name reaches stdout. Kept as one shared function (called by both backup and restore) so the sanitization is guaranteed identical on both paths.

## [0.8.10] - 2026-06-05

### Fixed
- Restore from a storage whose name contains a character invalid in a shell identifier (e.g. a hyphen, like `do-spaces`) failed: `duplicacy-restore.sh` built `DUPLICACY_<NAME>_S3_ID`/`_SECRET`/`_PASSWORD` from the **raw** name, so `export` rejected it ("not a valid identifier"), the credentials never reached duplicacy, and it fell through to an interactive prompt → EOF → that target was skipped. Now sanitizes the storage name (matching `duplicacy-backup.sh`) before building the env-var names and `-storage-name`.

## [0.8.9] - 2026-06-04

### Fixed
- `auto-restore-all.sh` shipped without its execute bit (mode 0644 in the repo, preserved into the image by `COPY`), so `archiver auto-restore-all` failed with "Permission denied" when the dispatcher `exec`'d it — breaking the tier-3 full-host restore path. Restored the script's exec bit and hardened the Dockerfile to `chmod +x lib/scripts/*.sh` so a missing source bit can't recur.

## [0.8.8] - 2026-06-03

### Added
- `RUN_RESTORE_SERVICE` env toggle on `auto-restore` (and, via pass-through, `auto-restore-all`): when non-empty, runs the service's `./restore-service.sh` after a successful file restore — non-interactively reloading databases and restarting the stack, mirroring what the interactive `restore` command prompts for. Its exit code propagates, so a failed reload surfaces as a failed restore (and a failed service in the `auto-restore-all` summary). Required for full-host DR where databases are backed up as dumps (not raw data dirs) and so must be reloaded post-restore. The interactive `restore` command also honors the toggle to skip its y/N prompt.

## [0.8.7] - 2026-06-03

### Added
- `auto-restore-all` command: non-interactive restore of every service in `SERVICE_DIRECTORIES` in one pass. Iterates the service registry and runs `auto-restore` at the latest revision (trying all storage targets) for each, then aggregates — exits 0 only if every service restored, 1 if any failed, 3 if the backup lock is held. Optional env (`REVISION`, `STORAGE_TARGET`, `OVERWRITE`, `DELETE_EXTRA`, `HASH_COMPARE`, `IGNORE_OWNERSHIP`) passes through to each per-service restore. Available as `archiver auto-restore-all` and entrypoint `run auto-restore-all`. Built for full-host disaster recovery onto a blank box.

### Dependencies

- chore(deps): update actions/checkout action to v6.0.3
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to c7d3c51
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 159006f
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 2bfc4bc
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 8f55827
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 5a74dfa
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 8c42892
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to fd4978c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 61e442c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 684d92c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 59ebcd2
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 49e7bdf
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4d6a1f3
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to bc173ba
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 96db676
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 9d905e2
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to b63ff3b
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 179b9ff
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4139c95
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4d60d6c

## [0.8.6] - 2026-05-05 - CI and Dockerfile modernization

No behavioral changes to the archiver application itself — only the CI pipeline, release flow, and image build process. Aligns archiver with the same externalized-repo conventions used by sibling projects on `forgejo.bryantserver.com`.

### Added
- `release.yml` workflow for cutting releases via `workflow_dispatch` with `bump=patch|minor|major`. Promotes `[Unreleased]` to a versioned section, bumps image refs in `compose.yaml` and `README.md`, runs a smoke test, then tags + pushes — replaces the prior manual `git tag` flow.
- `.renovaterc.json` introduced. Tracks the Dockerfile base image, ENV-pinned upstream versions (`DUPLICACY_VERSION` via github-releases against `gilbertchen/duplicacy`, `DOCKER_CLI_VERSION` via github-releases against `moby/moby`), Forgejo Action versions, and the inline skopeo image referenced by `docker-publish.yml`.

### Changed
- CI: `docker-publish.yml` split from a monolithic `build-and-push` job into separate jobs (`shellcheck`, `validate-changelog`, `pr-validate`, `build-and-push`, `create-release`, `summary`). Each concern has its own gate; failures surface to the new aggregator job rather than silently passing.
- CI: cluster push now builds to a local OCI archive and uses `skopeo copy` per tag rather than `docker buildx build --push`. Forgejo 15.0.1 nil-derefs in `EndUploadBlob` when buildx's parallel multi-arch push hits the `UQE_package_blob_blake2b`/`sha512` unique constraint — byte-identical blobs across amd64/arm64 (e.g. arch-independent `COPY` layers) race two PUTs of the same digest. Skopeo walks manifests serially with `TryReusingBlob` HEAD-checks, so byte-identical blobs across arches dedupe before PUT.
- CI: NAS Forgejo mirror now uses `skopeo sync` (full repo, self-healing) instead of `docker buildx imagetools create`. Same Forgejo 15.0.1 bug class as the cluster push.
- CI: GitHub Container Registry mirror now uses `skopeo copy` per tag instead of `docker buildx imagetools create`. Different registry (no Forgejo nil-deref bug there), but skopeo's HEAD-first dedup is safer against any concurrent-PUT race in the GHCR backend.
- CI: build cache moved from `type=registry,ref=...:buildcache` (separate cache push that hits the same Forgejo bug) to `type=inline` (cache annotations embedded on the published image manifest; `cache-from: ...:0.8` reads the previous release's inline cache on subsequent builds).
- CI: skopeo invocation stages the OCI archive into a docker-managed named volume via `docker cp` rather than bind-mounting from `${{ github.workspace }}` — the runner-container's view of the workspace path differs from the docker host's, so a child container's `-v <archive>:<target>` silently creates an empty dir at the host path instead of finding the archive.
- CI: release creation now reconciles every `v*.*.*` tag on every release run across all three registries (cluster Forgejo, NAS Forgejo, GitHub). A release that failed to publish on any registry (transient 5xx, NAS down, mirror lag) is picked up by the next successful release run rather than staying absent forever. Includes per-tag `target_commitish` from `git rev-parse "${tag}^{}"` for metadata normalization and defense against mirror-lag races where a reconciled tag isn't yet on the destination.
- CI: cluster Forgejo release is now attributed to a real-user PAT (`CLUSTER_REPO_WRITE_PAT`) instead of the auto-injected `GITHUB_TOKEN`, so release commits and tags carry a stable author identity rather than appearing as "ghost".
- CI: secret naming aligned with the bryantserver externalized-repo convention. `BRYANTSERVER_REGISTRY_PAT` → `CLUSTER_REGISTRY_PUSH_PAT`, `NAS_FORGEJO_PAT` → `NAS_FORGEJO_WRITE_PAT`. New: `CLUSTER_REPO_WRITE_PAT` for `release.yml`'s commit/tag/push and create-release attribution. `GHCR_PUSH_PAT` and `GH_RELEASE_PAT` unchanged (already convention-shaped).
- CI: tag patterns reduced to SemVer-only (was also `:main`/`:pr-N` from `type=ref,event=branch`/`event=pr`). PR builds now validate via the `pr-validate` job (build + smoke + arm64 dry-run) without polluting the registry with branch-name and PR-number tags.
- CI: Forgejo Action versions digest-pinned (`@v4`/`@v5` → `@<digest>`); Renovate tracks both major version and digest.
- Dockerfile: base image digest-pinned (`debian:trixie-20260112-slim` → `...@sha256:...`).
- Dockerfile: removed `wget`, `gnupg`, and `lsb-release` from the apt install list. No longer needed once curl replaces wget for the duplicacy download and the docker-ce-cli apt repo dance is gone.
- Dockerfile: Docker CLI install switched from the `docker-ce-cli` debian package (with `epoch:upstream-revision` versioning that has no clean Renovate datasource) to the static binary archive at `download.docker.com/linux/static/stable/<arch>/docker-<version>.tgz`. ENV pin becomes clean SemVer (`29.4.2` instead of `5:29.1.4-1`), Renovate-tracked via `extractVersion=^docker-v(?<version>.+)$` against `moby/moby` GitHub releases. Trade-off: lose apt's auto security-patch flow within a release stream; gain explicit Renovate-managed bumps. Acceptable for a CLI talking to a host-mounted socket.
- Dockerfile: `DUPLICACY_VERSION` Renovate-tracked via `extractVersion=^v(?<version>.+)$` against `gilbertchen/duplicacy` GitHub releases. Renovate auto-opens PRs for new duplicacy releases.
- Dockerfile: switched `wget` → `curl` for the duplicacy binary download, since curl is already installed and wget is no longer needed.

### Notes
- All 27 prior releases (v0.1.0 through v0.8.5) will be retroactively reconciled on the cluster + NAS Forgejos and GitHub on the next release run to set per-tag `target_commitish`. They previously had blank or `"main"` values; cosmetic backfill, no functional change.

## [0.8.5] - 2026-04-30 — Forgejo as primary registry

### Changed
- **Distribution**: Container images are now published to both `forgejo.bryantserver.com/sisyphusmd/archiver` and `ghcr.io/sisyphusmd/archiver`. Forgejo is the primary registry; `ghcr.io` remains available as a mirror. Existing image tags through 0.8.4 are unchanged on both registries.
- **Repository**: Project source-of-truth has moved to [forgejo.bryantserver.com/SisyphusMD/archiver](https://forgejo.bryantserver.com/SisyphusMD/archiver). The GitHub repository at [github.com/SisyphusMD/archiver](https://github.com/SisyphusMD/archiver) is now a read-only push-mirror.
- **Documentation**: README and configuration guides updated to reference the Forgejo image path as primary.

## [0.8.4] - 2026-04-24

### Changed
- **`archiver run backup` CLI now exits `1` on per-service errors.** Previously exited 0 regardless, surfacing failures only via log output and the optional Pushover notification. This makes the CLI suitable for external schedulers that key off the exit code — e.g., Kubernetes `Job` / `CronJob` (where the `Complete` / `Failed` status condition is derived from container exit code) or CI pipelines. The long-lived container deployment (`archiver start`, with the internal cron) is unaffected: per-service iteration and notification behavior are unchanged, and the backgrounded backup cycle's exit code remains unobserved by the daemon.

### Migration
- Shell wrappers around `archiver run backup` that relied on exit 0 and chained subsequent commands: wrap with `|| true` if the old continue-on-error behavior is desired.
- No action needed for containerized `archiver start` deployments.

## [0.8.3] - 2026-04-23

### Fixed
- **Container Detection in Kubernetes**: The container-deployment guard now recognizes Kubernetes Pods running on containerd or CRI-O, which create neither `/.dockerenv` nor `/run/.containerenv`. Detection now also checks `$KUBERNETES_SERVICE_HOST` (set automatically in every Pod) and `/proc/self/cgroup` (catches standalone containerd, CRI-O, LXC, rkt). Previously, running `archiver:0.8.2 run backup` from a Kubernetes `CronJob` or `Job` failed at startup with "Only container deployment is supported" unless the image was rebuilt with a `touch /.dockerenv` kludge.

### Changed
- **Internal**: Renamed `lib/core/require-docker.sh` → `lib/core/require-container.sh` and the associated `REQUIRE_DOCKER_CORE` / `REQUIRE_DOCKER_SH_SOURCED` constants. No user-visible change; the script is sourced internally.

## [0.8.2] - 2026-04-21

### Added
- **`run backup` Mode**: New synchronous backup path for external schedulers (Kubernetes `CronJob`, GitHub Actions scheduled workflows, systemd timers, etc.). `docker run ... archiver:0.8.2 run backup` decrypts the bundle, runs a backup synchronously, and exits with the backup's result code so the scheduler can report success/failure accurately. Accepts the same optional `prune` / `retain` flags as `archiver start`. This complements — and does not replace — `archiver start`, which remains async and is the right choice for the in-container cron daemon and for fire-and-forget use from a long-lived container.

## [0.8.1] - 2026-04-21

### Added
- **`run` Entrypoint Mode**: New container mode for one-shot non-interactive command invocation, designed for Kubernetes Jobs / init containers and other CI flows. `docker run ... archiver:0.8.1 run <subcommand>` decrypts the bundle, execs the subcommand, and the container's exit code equals the subcommand's exit code. Whitelisted subcommands: `auto-restore`, `snapshot-exists`, `healthcheck`. Long-running or async commands (`start`, `stop`, `pause`, etc.) are intentionally rejected with exit code `2`.

### Changed
- **Internal**: Renamed `lib/features/duplicacy.sh` to `lib/features/duplicacy-backup.sh` for symmetry with `lib/features/duplicacy-restore.sh` (introduced in 0.8.0). No user-visible change.
- **Internal**: Extracted bundle decrypt + import logic in `docker-entrypoint.sh` into a `prepare_bundle()` helper, reused by both the default daemon path and the new `run` mode. Behavior of existing `init` and daemon modes is unchanged.

## [0.8.0] - 2026-04-21

### Added
- **Non-interactive Restore Commands**: Two new subcommands for automated disaster recovery flows (e.g. Kubernetes init containers)
  - `archiver snapshot-exists` — probes every configured storage target for `SNAPSHOT_ID` and short-circuits on the first hit; exit codes `0=exists`, `1=not found`, `2=undetermined`, `3=lock held`
  - `archiver auto-restore` — iterates storage targets in order and restores from the first target that has the requested snapshot; env-driven (`SNAPSHOT_ID`, `LOCAL_DIR` required; `REVISION`, `STORAGE_TARGET`, `OVERWRITE`, `DELETE_EXTRA`, `HASH_COMPARE`, `IGNORE_OWNERSHIP`, `RESTORE_THREADS` optional)
- **Shared Restore Library**: Extracted duplicacy restore plumbing (storage target resolution, repo init, revision listing, restore) into `lib/features/duplicacy-restore.sh`, shared by interactive `restore`, `snapshot-exists`, and `auto-restore`

### Fixed
- **`archiver healthcheck` Exit Code**: The dispatcher previously clobbered the healthcheck's exit status to `0`, causing Docker healthchecks based on `archiver healthcheck` to always report healthy. It now propagates the underlying exit code

## [0.7.1] - 2026-04-14

### Added
- **Podman Support**: Container detection now recognizes Podman (`/run/.containerenv`) in addition to Docker (`/.dockerenv`), removing the need to mount a fake `.dockerenv` file
- **Host Management Tools**: Added `systemd` and `zfsutils-linux` packages to the container image
  - `systemctl` — manage host services from restore scripts (start, stop, mask, unmask, etc.)
  - `zfs` — take ZFS snapshots before restore operations (requires `/dev/zfs` mount)
  - Requires `SYSTEMCTL_FORCE_BUS=1` env var and D-Bus socket + systemd unit directory mounts
- **Security Hardening**: Added `cap_drop: ALL` with explicit `cap_add` to compose.yaml and README examples
  - `DAC_OVERRIDE` — required for writing to directories owned by other UIDs
  - `SETGID` — required for cron to execute scheduled jobs
  - `no-new-privileges:true` — recommended security option
- **Socket Documentation**: Documented mounting options for Podman socket, systemd D-Bus socket, systemd unit directory, and ZFS device node with per-socket security warnings

### Changed
- Updated available tools list in example scripts and migration guide to include `systemctl` and `zfs`

## [0.7.0] - 2026-01-22

### ⚠️ BREAKING CHANGES
- **Docker-Only Deployment**: Direct installation on host systems is no longer supported. See [migration guide](docs/guides/migration/legacy-to-docker.md).

### Added
- **Graceful Stop**: Stop command respects backup stage, completing service cleanup before termination
  - New `archiver stop --immediate` flag for emergency termination
  - Sends summary notification with runtime and error count
- **Docker Compose Graceful Shutdown**: Added `stop_grace_period: 2m` to ensure proper service cleanup when stopping containers
- **Migration Documentation**: Comprehensive guides for migrating legacy installations (v0.3.2+) to Docker v0.7.0
  - Legacy to Docker migration guide
  - Configuration editing in Docker
  - Local storage setup
  - SSH key management

### Improved
- **Prune Operation**: Now uses `-exhaustive` flag to remove orphaned chunks from manually deleted snapshots and incomplete backups
- **Notifications**: Include hostname and timestamp; respect TZ environment variable
- **Bundle Export**: Offers to reuse BUNDLE_PASSWORD environment variable
- **Storage Names**: Automatically sanitize storage names with hyphens for Bash compatibility
- **Code Organization**: Restructured lib directory into core/features/scripts

### Changed
- Streamlined documentation for Docker-only workflow
- **SFTP SSH Key Path**: Hardcoded to `/opt/archiver/keys/id_ed25519` for Docker consistency
  - Removed `STORAGE_TARGET_X_SFTP_KEY_FILE` configuration variable
  - Migrating users: Old variable in config.sh will be safely ignored

### Removed
- Legacy/direct installation support and related commands

## [0.6.5] - 2026-01-14

### Added
- **Container Tools**: Added text editors and network utilities to Docker image
  - `nano` - Simple text editor for quick file edits
  - `vim` - Full-featured text editor
  - `iputils-ping` - Network connectivity testing

### Changed
- **Documentation**: Updated README to prioritize `archiver logs` command over `docker logs`
- **Examples**: Updated service script examples with complete list of available tools

## [0.6.4] - 2026-01-14

### Fixed
- **Docker Container Issues**:
  - Added `procps` package to Docker image to provide `pkill` and `pgrep` commands
  - Fixes "command not found" errors when stopping backup processes
  - Resolves container shutdown hangs caused by orphaned processes
  - Improved graceful shutdown by explicitly terminating log tailer process
- **Bundle Export**: Fixed bundle backup verification to prevent data loss
  - Now verifies the backup file exists before continuing with export
  - Exits with error if backup operation fails instead of proceeding
- **Docker Logs**: Changed log tailer to show only new logs (`tail -n 0`)
  - Prevents `docker logs` from hanging with large log files (100k+ lines)
  - Improves performance for long-running backups with extensive logging

### Changed
- **Docker Volume Mount**: Updated documentation to mount bundle directory instead of single file
  - Resolves "Device or resource busy" errors during bundle export
  - Allows proper file operations within mounted directory

## [0.6.3] - 2026-01-13

### Changed
- **Duplicacy Update**: Updated Duplicacy to v3.2.5 (from v3.2.3)

## [0.6.2] - 2026-01-13

### Added
- **SQLite3 Support**: sqlite3 package now included in container image
  - Enables database operations and backups directly from service scripts
  - Useful for services using SQLite databases

### Documentation
- Added comprehensive list of available tools and packages in example service scripts
- Documents all available tools: duplicacy, docker, sqlite3, curl, wget, ssh, openssl, etc.
- Clarifies Docker socket requirement for docker commands

## [0.6.1] - 2026-01-13

### Added
- **Docker CLI Support**: Docker CLI now included in container image
  - Enables service-specific backup scripts to control other Docker containers
  - Version-locked to Docker CLI 29.1.4-1 for reproducible builds
  - Required for backup scripts that need to exec commands in other containers or manage container state

### Documentation
- Added requirement to mount Docker socket (`/var/run/docker.sock`) for service scripts that need Docker access
- Added security warning about Docker socket access implications

## [0.6.0] - 2026-01-13

### Added
- **Local Disk Storage Backend**: Support for local disk as a storage target alongside SFTP, B2, and S3
  - Ideal as primary backup target with fast local backups
  - Can be combined with remote storage for off-site redundancy
- **Performance Optimization**: Configurable thread count for duplicacy operations
  - New `DUPLICACY_THREADS` configuration variable (default: 4)
  - Parallel upload/download threads for faster backups
  - Applied to backup, copy, restore, check, and prune operations
- **Interactive Restore Options**: Advanced restore configuration wizard
  - Hash-based file detection option
  - Overwrite existing files option
  - Delete files not in snapshot option
  - Ignore ownership option
  - Continue on errors option
  - Customizable thread count for restore operations
- **Graceful Container Shutdown**: Docker containers handle SIGTERM properly
  - Running backups are stopped gracefully on `docker stop`
  - Prevents data corruption from forced termination

### Improved
- Restore operations now have configurable performance and behavior options
- Backup operations complete faster with parallel threading
- Docker container lifecycle management with proper signal handling

## [0.5.1] - 2026-01-13

### Added
- SSH public key display after bundle export for easy SFTP server configuration

### Fixed
- Symlink for `archiver.log` now uses relative path, resolving correctly when logs directory is mounted outside container
- Bundle backup (`.old` file) handling improved to remove existing backup before creating new one

## [0.5.0] - 2026-01-11

### Added
- **Docker Support**: Run Archiver in containers on any platform
  - Interactive setup mode generates bundle file without Linux installation
  - Automated backups with cron scheduling or manual execution
  - Multi-architecture images (linux/amd64, linux/arm64) published to GHCR
- **Bundle Commands**: `archiver bundle export` and `archiver bundle import` for managing configuration
- **Health Check**: `archiver healthcheck` command for monitoring system health

### Changed
- **BREAKING**: Renamed export/import to bundle terminology
  - `archiver export` → `archiver bundle export`
  - `archiver import` → `archiver bundle import`
  - Directory: `exports/` → `bundle/`
  - Environment: `EXPORT_PASSWORD` → `BUNDLE_PASSWORD`
- Bundle file is now always `bundle.tar.enc` (previous version saved as `.old`, no timestamps)
- Cron jobs now scheduled for invoking user instead of root user
- Logs now show storage target name (e.g., "Service: Synology") instead of generic "Archiver" during copy/wrap-up operations

### Fixed
- Setup script now handles special characters (like `$`) in passwords and API keys correctly
- Setup script now creates bundle file even if one wasn't imported
- Setup script improved S3 region prompt with better examples and optional handling
- Setup script and bundle-export script now work correctly in Docker (no SUDO_USER dependency)
- Cron schedule now properly created from setup script for the correct user
- Log viewer now properly exits when backup completes instead of hanging
- Restored files now have correct ownership matching the invoking user
- Directory ownership fixed when restoring to non-existent directory
- Removed obsolete `PRUNE_KEEP_ARRAY` from log output
- Better error messages when running setup on unsupported platforms

### Improved
- Setup script auto-imports existing bundle files
- Version pinning for reliable installations
- Platform detection provides helpful Docker guidance for non-Linux systems
- Cron instructions now show correct commands for user crontab

### Migration Notes

#### Migrating from v0.4.x to v0.5.0 (Bundle Terminology)

The export/import functionality has been renamed to use "bundle" terminology:

1. **Export file location changed**: `exports/` directory is now `bundle/`
2. **Commands renamed**:
   - `archiver export` → `archiver bundle export`
   - `archiver import` → `archiver bundle import`
3. **Bundle file naming**: Files are now always named `bundle.tar.enc` (previous versions saved as `.old`, no timestamps)
4. **Environment variable**: For Docker, `EXPORT_PASSWORD` is now `BUNDLE_PASSWORD`

**Action Required for existing export files**:
- Rename your `exports/` directory to `bundle/`
- Rename your export file to `bundle.tar.enc`
- Use the new `archiver bundle export` and `archiver bundle import` commands going forward

#### Migrating from Traditional Installation to Docker

Docker installation is now the recommended deployment method. Legacy installation will be **deprecated in v0.7.0**.

**Steps to migrate**:

1. **Export your configuration** (from your current traditional install):
   ```bash
   archiver bundle export
   ```
   This creates `bundle/bundle.tar.enc` with your config and keys encrypted.

2. **Save your bundle password**: Remember the password you used to encrypt the bundle - you'll need it for Docker.

3. **Set up Docker Compose**:
   ```yaml
   services:
     archiver:
       container_name: archiver
       image: ghcr.io/sisyphusmd/archiver:0.5.0
       restart: unless-stopped
       environment:
         TZ: "UTC" # Examples: "America/New_York", "Europe/London", "Asia/Tokyo"
         BUNDLE_PASSWORD: "your-bundle-password-here"
         CRON_SCHEDULE: "0 3 * * *"  # Optional - backup will wait for manual invocation if not set
       volumes:
         - /path/to/bundle.tar.enc:/opt/archiver/bundle/bundle.tar.enc
         - /path/to/logs:/opt/archiver/logs # Optional - for log persistence
         - /path/to/service_directories:/path/to/service_directories # Container path must match SERVICE_DIRECTORIES in your config.sh
       hostname: backup-server # Optional - used in backup snapshot IDs
   ```

4. **Important**: Volume paths inside the container must match the `SERVICE_DIRECTORIES` paths in your `config.sh`. If your config has `/home/user/data`, mount it to the same path: `-/home/user/data:/home/user/data`

5. **Start the container**:
   ```bash
   docker compose up -d
   ```

6. **Verify**: Check logs with `docker logs -f archiver`

**Note**: You can run both traditional and Docker installations side-by-side during migration for testing.

## [0.4.1] - 2025-06-07
### Fixed
- Added missing restore function for S3 storage backend.
- Fixed ownership of new local directory when restoring.

## [0.4.0] - 2025-06-06
### Added
- S3 storage backend support added alongside existing SFTP and B2 backends.

### Improved
- Setup script explicitly checks for export file existence, skipping import gracefully if no exports exist.

## [0.3.2] - 2024-06-06
### Improved
- Added import and export functions to backup your config.sh and keys files
- Setup script will auto-import any export file placed in the archiver repo directory
- Setup script also ensures you have an export file, and places it in an 'exports' directory

## [0.3.1] - 2024-06-06
### Fixed
- Fixed pruning (backup rotations)

## [0.3.0] - 2024-06-05
## **!!!BREAKING CHANGE!!!**
- Calling the script with no argument will no longer initiate a backup
- Must use the full command with argument: 'archiver start'
- If you have cronjobs scheduled without the 'start' argument, they will no longer initiate a backup without editing to include 'start'
  - You can edit your cronjobs with the following command: 'sudo crontab -e'
  - i.e. '0 3 * * * archiver start'

### Improved
- Massive argument improvements:
  - Arguments are now single words, not prefaced by '--'
    - archiver start|stop|pause|resume|restart|logs|status|setup|uninstall|restore|help
  - 'archiver' command with no argument (or with 'help' argument) prints a guide to available arguments
  - 'start':
    - 'archiver start' is now required to initiate a backup
      - may need to edit 'sudo crontab -e' if it previously did not include the 'start' argument
    - 'archiver start logs' to initiate a backup and view logs
    - 'archiver start prune|retain' prune and retain will override the behavior to prune or retain backups for this run only
    - logs and prune|retain can be combined
  - 'stop|pause|resume':
    - 'archiver stop|pause|resume' manually stops, pauses, or resumes a running backup
    - 'archiver resume' can be combined with 'logs'
  - 'logs'
    - 'archiver logs' will display the logs of a running backup (but no longer starts a new backup)
  - 'status'
    - 'archiver status' prints whether or not there is a currently running backup process
  - 'restart'
    - 'archiver restart' will stop any running backup and start a new one from the beginning
    - similar to 'archiver start' can be used with logs|prune|retain
  - 'restore'
    - 'archiver restore' will run the restore script
  - 'help'
    - 'archiver help' will display information about available commands and arguments
  - 'setup|uninstall'
    - 'archiver setup|uninstall' will run the setup or uninstall scripts
    - although, on first run, setup will require './archiver.sh setup' from the archiver dir, given archiver will not be in the PATH yet
    - uninstall function coming soon

## [0.2.3] - 2024-06-04
### Fixed
- Fixed LOCKFILE being left by main.sh.

### Improved
- Setup script places archiver in PATH.
  - Please run './setup.sh' again from the archiver repo directory to make this change.
    - You can skip all sections of the setup script by typing 'n' when prompted. The script will make this change regardless.
  - You should now run Archiver backups with the command 'archiver'.
  - This is global, no more need to change to your archiver directory.
  - It accepts arguments, such as 'archiver --view-logs' and 'archiver --stop'. More arguments to come soon.
  - Cron can also call 'archiver' directly: (e.g. '0 3 * * * archiver').
    - To edit your prior cronjob, run 'sudo crontab -e', and replace the path to the archiver script with simply 'archiver'.

## [0.2.2] - 2024-06-03
### Improved
- Scripts will auto-escalate to sudo now. So README no longer recommends to run with sudo.
- Logs all go to a single file now for easier viewing.
- Reorganized directory structure.
- Stop is now an argument './archiver.sh --stop'.

## [0.2.1] - 2024-06-02
### Improved
- The LOCKFILE mechanism is much more robust now.
- Setting the stage for all commands to be through "sudo archiver --argument" rather than through calling various scripts.

## [0.2.0] - 2024-06-01
### Improved
- Major improvements to speed. Backup to primary storage for all repositories completes first, then each secondary storage copies sequentially.

## [0.1.1] - 2024-06-01
### Fixed
- Fixed the Duplicacy Prune function. Backup rotations work now.

## [0.1.0] - 2024-05-31
### Added
- Initial release of the project.
