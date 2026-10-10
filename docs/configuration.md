# Configuration reference

<!-- Generated from internal/config/reference.go by `UPDATE_REFERENCE=1 go test ./internal/config`. Do not edit. -->

Settings are environment variables. **Secrets** are files, never environment variables: `<secrets dir>/<name in lower case>` (`/run/secrets/storage_password`), or the file `<NAME>_FILE` names. An editor can validate an env file against [archiver.schema.json](archiver.schema.json).

## Services

| Variable | Default | Description |
|---|---|---|
| `SERVICE_DIRECTORIES` |  | Directories to back up, separated by colons or newlines; each may be a bash glob such as `/srv/*/`. Each directory is one snapshot, `<hostname>-<directory name>`. |
| `BACKUP_PARALLELISM` | `2` | How many services a backup processes at once, each with its hooks and backup. 1 backs them up one after another. |
| `DUPLICACY_THREADS` | `4` | Threads each Duplicacy backup, copy, check and restore uses. |

## Schedules

| Variable | Default | Description |
|---|---|---|
| `BACKUP_SCHEDULE` |  | When backups run, as a cron expression (`0 3 * * *`). Unset: manual mode, nothing is scheduled. |
| `MAINTENANCE_SCHEDULE` |  | When storage check and prune run. Unset: they run only on `archiver maintenance`. |
| `RESTORE_DRILL_SCHEDULE` |  | When restore drills run (ADR 28). Unset: no drills. |
| `TZ` | `UTC` | Time zone for schedules and log timestamps, such as `America/New_York`. |

## Storage

| Variable | Default | Description |
|---|---|---|
| `STORAGE_PASSWORD` (secret) |  | Password encrypting every storage (at least 8 characters, a Duplicacy rule). |
| `RSA_PASSPHRASE` (secret) |  | Passphrase of the RSA private key that encrypts the backups. |

## Maintenance

| Variable | Default | Description |
|---|---|---|
| `CHECK_BACKUPS` | `true` | Whether maintenance checks the storages. Turn off on a deployment sharing a storage another one maintains. |
| `PRUNE_BACKUPS` | `true` | Whether maintenance prunes by PRUNE_KEEP. Turn off on a deployment sharing a storage another one maintains. |
| `ROTATE_BACKUPS` |  | Old name of PRUNE_BACKUPS, still read when PRUNE_BACKUPS is unset. |
| `PRUNE_KEEP` | `-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1` | Duplicacy retention: one revision a day for a week, a week for a month, a month for half a year, none older. |
| `PRUNE_EXHAUSTIVE_FREQUENCY` | `monthly` | How often the prune also removes chunks no revision references. One of `off`, `daily`, `weekly`, `monthly`. |
| `CHECK_INTERVAL` |  | How often each secondary is checked (`12h`, `7d`); a target's own STORAGE_TARGET_N_CHECK_INTERVAL wins. Default: a day, a week for SFTP-like storages. |

## Notifications

| Variable | Default | Description |
|---|---|---|
| `NOTIFY_ON` | `failures` | Which notifications every destination receives (ADR 36). One of `failures`, `problems`, `everything`. |
| `ALERT_REPEAT_INTERVAL` | `24h` | How often an ongoing failure is notified again (`6h`, `2d`); `0` never repeats (ADR 33). |
| `NOTIFICATION_SERVICE` |  | `Pushover` sends to Pushover with the secrets below (any letter case). One of `Pushover`, `None`. |
| `PUSHOVER_USER_KEY` (secret) |  | Pushover user key. |
| `PUSHOVER_API_TOKEN` (secret) |  | Pushover application token. |
| `PUSHOVER_NOTIFY_ON` |  | NOTIFY_ON for Pushover alone. One of `failures`, `problems`, `everything`. |
| `APPRISE_URL` (secret) |  | An Apprise API notify URL (`http://apprise:8000/notify/<key>`, with `user:password@` for basic auth). |
| `APPRISE_TAGS` |  | The Apprise tag for each kind, such as `failure=critical,problem=alerts,routine=quiet`; a kind left out goes to `all`. |
| `APPRISE_NOTIFY_ON` |  | NOTIFY_ON for Apprise alone. One of `failures`, `problems`, `everything`. |
| `NTFY_URL` |  | An ntfy server and topic, such as `https://ntfy.sh/my-archiver`. |
| `NTFY_TOKEN` (secret) |  | An ntfy access token, if the topic needs one. |
| `NTFY_NOTIFY_ON` |  | NOTIFY_ON for ntfy alone. One of `failures`, `problems`, `everything`. |

## Monitoring

| Variable | Default | Description |
|---|---|---|
| `CHECKIN_URL` |  | A dead man's switch pinged after each backup run (ADR 37): Uptime Kuma push URLs get `status=up`/`down`, others `/fail` on failure. |
| `METRICS_PORT` |  | Serves Prometheus metrics on `/metrics` there (ADR 35); `logs/archiver.prom` is written either way. |
| `WEB_PORT` |  | Serves the read-only status page there (ADR 38). It has no login: put it behind an authenticating proxy. |
| `LOG_FORMAT` | `text` | `json` writes the container's output as one JSON object per line. One of `text`, `json`. |

## Restore drills

| Variable | Default | Description |
|---|---|---|
| `RESTORE_DRILL_SERVICES` | `1` | How many services each drill restores from each storage, rotating; `all` restores every one. |
| `RESTORE_DRILL_STORAGES` | `all` | Which storages drills restore from: `all`, `primary`, or storage names separated by commas. |
| `RESTORE_DRILL_DIR` | `/tmp/archiver-drill` | Scratch space a drill restores into and deletes afterwards; never a service directory. |
| `RESTORE_DRILL_EXCLUDE` |  | Services drills leave out, separated by commas. |

## Recovery kit

| Variable | Default | Description |
|---|---|---|
| `RECOVERY_PASSWORD` (secret) |  | Encrypts the recovery kit uploaded beside the backups; without it there is no kit. Keep it in a password manager. |
| `RECOVERY_KIT_EXTRA_PATHS` |  | Files or directories also carried in the kit's `extra/`, separated by colons. |

## Hooks

| Variable | Default | Description |
|---|---|---|
| `HOOKS_DIR` |  | Hooks come from `<HOOKS_DIR>/<service>/` instead of the service directories (ADR 45). |

## Container

| Variable | Default | Description |
|---|---|---|
| `RSA_PRIVATE_KEY_FILE` | `/run/secrets/rsa_private_key` | The RSA private key to place in the container's key directory at start. |
| `RSA_PUBLIC_KEY_FILE` | `/run/secrets/rsa_public_key` | The RSA public key to place at start. |
| `SSH_PRIVATE_KEY_FILE` | `/run/secrets/ssh_private_key` | SFTP storages: the SSH private key to place at start. |
| `SSH_PUBLIC_KEY_FILE` | `/run/secrets/ssh_public_key` | SFTP storages: the SSH public key to place at start. |
| `HOSTNAME` |  | Overrides the host part of snapshot IDs and the kit's name. Keep it stable: changing it starts new snapshots. |
| `SECRETS_DIR` | `/run/secrets` | Where secret files are read from. Any secret can instead be named by `<SECRET>_FILE`. |

## Restore commands

| Variable | Default | Description |
|---|---|---|
| `SNAPSHOT_ID` |  | auto-restore and snapshot-exists: the snapshot, `<hostname>-<service>`. |
| `LOCAL_DIR` |  | auto-restore: where to restore. |
| `REVISION` | `latest` | auto-restore: a revision number or `latest`. |
| `STORAGE_TARGET` |  | auto-restore: restore from this storage (name or number) only. |
| `OVERWRITE` |  | auto-restore: replace files that differ. |
| `DELETE_EXTRA` |  | auto-restore: delete files the revision does not have. |
| `HASH_COMPARE` |  | auto-restore: compare files by content, not size and time. |
| `IGNORE_OWNERSHIP` |  | auto-restore: leave ownership alone (without CHOWN and FOWNER it is left anyway, with a warning). |
| `RESTORE_PATHS` |  | auto-restore: only these paths in the snapshot, separated by commas. |
| `DRY_RUN` |  | auto-restore: show what would be restored, replaced and deleted, and restore nothing. |
| `RUN_RESTORE_SERVICE` |  | auto-restore: run the service's post-restore hook after the files are restored. |
| `RESTORE_THREADS` |  | auto-restore: threads for this restore. Default: DUPLICACY_THREADS. |
| `NO_PREVIEW` |  | restore: skip the preview of what an interactive restore changes. |
| `RECOVERY_PASSWORD_FILE` |  | recover: a file holding the kit's password, instead of the prompt. |

## Storage targets

Each storage is `STORAGE_TARGET_<N>_*`, numbered from 1 without gaps; target 1 is the primary, the others are kept as copies of it. The variables a type uses (its fields) are listed with the types that use them.

| Variable | Types | Default | Description |
|---|---|---|---|
| `STORAGE_TARGET_<N>_NAME` | all |  | The storage's name. Target 1 is the primary; the others are copies of it. |
| `STORAGE_TARGET_<N>_TYPE` | all |  | The storage type. One of `azure`, `b2`, `b2-custom`, `dropbox`, `fabric`, `gcd`, `gcs`, `local`, `minio`, `minios`, `odb`, `one`, `s3`, `s3c`, `sftp`, `sftpc`, `smb`, `storj`, `swift`, `wasabi`, `webdav`, `webdav-http`. |
| `STORAGE_TARGET_<N>_CHECK_INTERVAL` | all |  | How often this secondary is checked; overrides CHECK_INTERVAL. |
| `STORAGE_TARGET_<N>_CHECKIN_URL` | all |  | Pinged when this storage is caught up or checked, its fail variant when copies or its check fail (ADR 37). |
| `STORAGE_TARGET_<N>_BREAKGLASS_SFTP_USER` | all |  | SFTP storages: the user the printed envelope names, for read-only access (ADR 23). |
| `STORAGE_TARGET_<N>_BREAKGLASS_SSH_KEY` (secret) | all |  | SFTP storages: the private key of BREAKGLASS_SFTP_USER, printed on the envelope (ADR 23). |
| `STORAGE_TARGET_<N>_AZURE_ACCOUNT` | azure |  | Azure storage account. |
| `STORAGE_TARGET_<N>_AZURE_CONTAINER` | azure |  | Azure container. |
| `STORAGE_TARGET_<N>_AZURE_KEY` (secret) | azure |  | Azure access key. |
| `STORAGE_TARGET_<N>_BREAKGLASS_AZURE_KEY` (secret) | azure |  | A narrower credential than AZURE_KEY (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_B2_BUCKETNAME` | b2, b2-custom |  | B2 bucket name. |
| `STORAGE_TARGET_<N>_B2_PATH` | b2, b2-custom |  | Path inside the bucket. Optional. |
| `STORAGE_TARGET_<N>_B2_ID` (secret) | b2, b2-custom |  | B2 key ID. |
| `STORAGE_TARGET_<N>_BREAKGLASS_B2_ID` (secret) | b2, b2-custom |  | A narrower credential than B2_ID (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_B2_KEY` (secret) | b2, b2-custom |  | B2 application key. |
| `STORAGE_TARGET_<N>_BREAKGLASS_B2_KEY` (secret) | b2, b2-custom |  | A narrower credential than B2_KEY (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_B2_DOWNLOAD_HOST` | b2-custom |  | B2 download host. |
| `STORAGE_TARGET_<N>_DROPBOX_PATH` | dropbox |  | Folder in the Dropbox (inside the app's folder for an app-folder app). |
| `STORAGE_TARGET_<N>_DROPBOX_APP_KEY` | dropbox |  | App key of your Dropbox app. |
| `STORAGE_TARGET_<N>_DROPBOX_APP_SECRET` (secret) | dropbox |  | App secret of your Dropbox app. |
| `STORAGE_TARGET_<N>_DROPBOX_TOKEN` (secret) | dropbox |  | Refresh token. |
| `STORAGE_TARGET_<N>_FABRIC_ENDPOINT` | fabric |  | File Fabric host. |
| `STORAGE_TARGET_<N>_FABRIC_PATH` | fabric |  | Path. Optional. |
| `STORAGE_TARGET_<N>_FABRIC_TOKEN` (secret) | fabric |  | File Fabric permanent token. |
| `STORAGE_TARGET_<N>_BREAKGLASS_FABRIC_TOKEN` (secret) | fabric |  | A narrower credential than FABRIC_TOKEN (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_GCD_PATH` | gcd |  | Path in the drive. Optional. |
| `STORAGE_TARGET_<N>_GCD_DRIVE` | gcd |  | Shared drive ID, from its URL (empty for My Drive). Optional. |
| `STORAGE_TARGET_<N>_GCD_TOKEN` (secret) | gcd |  | Token file from your own OAuth app, or a service account file. |
| `STORAGE_TARGET_<N>_GCS_BUCKETNAME` | gcs |  | GCS bucket name. |
| `STORAGE_TARGET_<N>_GCS_PATH` | gcs |  | Path inside the bucket. Optional. |
| `STORAGE_TARGET_<N>_GCS_TOKEN` (secret) | gcs |  | Service account (JSON) file. |
| `STORAGE_TARGET_<N>_LOCAL_PATH` | local |  | Local path. |
| `STORAGE_TARGET_<N>_S3_BUCKETNAME` | minio, minios, s3, s3c |  | S3 bucket name. |
| `STORAGE_TARGET_<N>_S3_ENDPOINT` | minio, minios, s3, s3c |  | S3 endpoint (host[:port]). |
| `STORAGE_TARGET_<N>_S3_REGION` | minio, minios, s3, s3c | `none` | S3 region. Optional. |
| `STORAGE_TARGET_<N>_S3_PATH` | minio, minios, s3, s3c |  | Path inside the bucket. Optional. |
| `STORAGE_TARGET_<N>_S3_ID` (secret) | minio, minios, s3, s3c |  | S3 access key ID. |
| `STORAGE_TARGET_<N>_BREAKGLASS_S3_ID` (secret) | minio, minios, s3, s3c |  | A narrower credential than S3_ID (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_S3_SECRET` (secret) | minio, minios, s3, s3c |  | S3 secret key. |
| `STORAGE_TARGET_<N>_BREAKGLASS_S3_SECRET` (secret) | minio, minios, s3, s3c |  | A narrower credential than S3_SECRET (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_ODB_PATH` | odb |  | Path in the drive. Optional. |
| `STORAGE_TARGET_<N>_ODB_DRIVE_ID` | odb |  | Drive ID (empty for your own drive). Optional. |
| `STORAGE_TARGET_<N>_ODB_CLIENT_ID` | odb |  | Client ID of your app registration. |
| `STORAGE_TARGET_<N>_ODB_CLIENT_SECRET` (secret) | odb |  | Client secret of your app registration. |
| `STORAGE_TARGET_<N>_ODB_TOKEN` (secret) | odb |  | Token file. |
| `STORAGE_TARGET_<N>_ONE_PATH` | one |  | Path in the drive. |
| `STORAGE_TARGET_<N>_ONE_CLIENT_ID` | one |  | Client ID of your app registration. |
| `STORAGE_TARGET_<N>_ONE_CLIENT_SECRET` (secret) | one |  | Client secret of your app registration. |
| `STORAGE_TARGET_<N>_ONE_TOKEN` (secret) | one |  | Token file. |
| `STORAGE_TARGET_<N>_SFTP_URL` | sftp, sftpc |  | SFTP host (IP or FQDN). |
| `STORAGE_TARGET_<N>_SFTP_PORT` | sftp, sftpc | `22` | SFTP port. |
| `STORAGE_TARGET_<N>_SFTP_USER` | sftp, sftpc |  | SFTP user. |
| `STORAGE_TARGET_<N>_SFTP_PATH` | sftp, sftpc |  | SFTP path. |
| `STORAGE_TARGET_<N>_SMB_HOST` | smb |  | SMB host[:port]. |
| `STORAGE_TARGET_<N>_SMB_USER` | smb |  | SMB user. |
| `STORAGE_TARGET_<N>_SMB_SHARE` | smb |  | SMB share. |
| `STORAGE_TARGET_<N>_SMB_PATH` | smb |  | Path inside the share. Optional. |
| `STORAGE_TARGET_<N>_SMB_PASSWORD` (secret) | smb |  | SMB password. |
| `STORAGE_TARGET_<N>_BREAKGLASS_SMB_PASSWORD` (secret) | smb |  | A narrower credential than SMB_PASSWORD (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_STORJ_SATELLITE` | storj |  | Storj satellite (host:port). |
| `STORAGE_TARGET_<N>_STORJ_BUCKET` | storj |  | Storj bucket. |
| `STORAGE_TARGET_<N>_STORJ_PATH` | storj |  | Path inside the bucket. Optional. |
| `STORAGE_TARGET_<N>_STORJ_KEY` (secret) | storj |  | Storj API key. |
| `STORAGE_TARGET_<N>_BREAKGLASS_STORJ_KEY` (secret) | storj |  | A narrower credential than STORJ_KEY (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_STORJ_PASSPHRASE` (secret) | storj |  | Storj encryption passphrase. |
| `STORAGE_TARGET_<N>_SWIFT_URL` | swift |  | Swift URL (user@auth-host/v3/container[/path][?domain=…&tenant=…]). |
| `STORAGE_TARGET_<N>_SWIFT_KEY` (secret) | swift |  | Swift key (password). |
| `STORAGE_TARGET_<N>_BREAKGLASS_SWIFT_KEY` (secret) | swift |  | A narrower credential than SWIFT_KEY (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_WASABI_BUCKETNAME` | wasabi |  | Wasabi bucket name. |
| `STORAGE_TARGET_<N>_WASABI_ENDPOINT` | wasabi | `s3.wasabisys.com` | Wasabi endpoint. |
| `STORAGE_TARGET_<N>_WASABI_REGION` | wasabi | `us-east-1` | Wasabi region. |
| `STORAGE_TARGET_<N>_WASABI_PATH` | wasabi |  | Path inside the bucket. Optional. |
| `STORAGE_TARGET_<N>_WASABI_KEY` (secret) | wasabi |  | Wasabi access key. |
| `STORAGE_TARGET_<N>_BREAKGLASS_WASABI_KEY` (secret) | wasabi |  | A narrower credential than WASABI_KEY (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_WASABI_SECRET` (secret) | wasabi |  | Wasabi secret key. |
| `STORAGE_TARGET_<N>_BREAKGLASS_WASABI_SECRET` (secret) | wasabi |  | A narrower credential than WASABI_SECRET (read-only), printed on the envelope instead of it (ADR 23). Optional. |
| `STORAGE_TARGET_<N>_WEBDAV_HOST` | webdav, webdav-http |  | WebDAV host[:port]. |
| `STORAGE_TARGET_<N>_WEBDAV_USER` | webdav, webdav-http |  | WebDAV user. |
| `STORAGE_TARGET_<N>_WEBDAV_PATH` | webdav, webdav-http |  | WebDAV path. |
| `STORAGE_TARGET_<N>_WEBDAV_PASSWORD` (secret) | webdav, webdav-http |  | WebDAV password. |
| `STORAGE_TARGET_<N>_BREAKGLASS_WEBDAV_PASSWORD` (secret) | webdav, webdav-http |  | A narrower credential than WEBDAV_PASSWORD (read-only), printed on the envelope instead of it (ADR 23). Optional. |
