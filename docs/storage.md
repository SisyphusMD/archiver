# Storage

Where backups go: the primary every backup writes to, and the secondaries kept as copies of it.

## Storage Backend Setup

Prepare at least one storage location before running init. Every other type Duplicacy supports is listed under [Storage types](#storage-types).

### Local Disk

Local disk storage is the simplest and fastest option, ideal as your primary backup target. Backups can then be copied from local storage to remote locations (SFTP, B2, S3) for off-site redundancy.

#### Requirements

- A local directory path (e.g., `/mnt/backup/storage`)
- Sufficient disk space for your backups
- Proper read/write permissions

#### Setup

1. Create the backup directory:
   ```bash
   sudo mkdir -p /mnt/backup/storage
   ```

2. Set appropriate permissions:
   ```bash
   sudo chown -R $USER:$USER /mnt/backup/storage
   sudo chmod 755 /mnt/backup/storage
   ```

3. Verify the directory is accessible:
   ```bash
   ls -la /mnt/backup/storage
   ```

**Tip:** For the 3-2-1 backup strategy, use local disk as your primary storage target for fast backups, then configure additional remote storage targets to copy backups off-site automatically.

### SFTP - Synology NAS

#### Enable SFTP

1. Login to Synology DSM Web UI (usually `http://<nas-ip>:5000`)
2. Open **Control Panel** → **File Services** → **FTP** tab
3. Enable **SFTP service** (not FTP/FTPS)
4. Default port **22** is fine
5. Click **Apply**

#### Create User

1. **Control Panel** → **User & Group** → **Create**
2. Set **Name** and **Password**
3. Assign to appropriate **Groups**
4. Grant shared folder permissions
5. Under **Application Permissions**, allow **SFTP**
6. Complete and click **Done**

#### Create Shared Folder

1. **Control Panel** → **Shared Folder** → **Create**
2. Name the folder and configure settings
3. Don't enable **WriteOnce** (incompatible with backups)
4. If using BTRFS, enable **data checksum**
5. Grant **Read/Write** access to backup user

#### Add SSH Key

Generate SSH key first (init script can do this), then:

1. **Control Panel** → **User & Group** → **Advanced**
2. Enable **user home service**
3. Open **File Station** → **homes** folder → your user folder
4. Create `.ssh` folder if it doesn't exist
5. Upload or create `authorized_keys` file containing your public key (`ssh-ed25519 AAAA...`)

### B2 - BackBlaze

#### Account Setup

1. [Create account](https://www.backblaze.com/sign-up/cloud-storage) or [sign in](https://secure.backblaze.com/user_signin.htm)
2. **My Settings** → Enable **B2 Cloud Storage**

#### Create Bucket

1. **Buckets** → **Create a Bucket**
2. Choose unique **Bucket Name**
3. Files: **Private**
4. Encryption: **Enable**
5. Object Lock: **Disable**
6. Lifecycle: **Keep all versions** (default)

#### Application Key

1. **Application Keys** → **Add New**
2. Name the key
3. Allow access to your bucket
4. Type: **Read and Write**
5. Enable **List All Bucket Names**
6. Click **Create New Key**
7. Save the **keyID** and **applicationKey** (shown only once)

### S3-Compatible Storage

S3 providers vary, but you'll need:

- **Bucket Name** (globally unique)
- **Endpoint** (e.g., `s3.amazonaws.com` or `s3.us-east-1.wasabisys.com`)
- **Region** (optional, provider-specific, e.g., `us-east-1`)
- **Access Key ID** (with read/write permissions)
- **Secret Access Key**

Create these through your S3 provider's console (AWS, Wasabi, Backblaze S3 API, etc.)

## Storage Targets

Define multiple storage locations of any [type](#storage-types):

> **Note:** Storage names should only contain letters, numbers, and underscores. Other characters will be automatically sanitized (e.g., `my-storage` → `my_storage`).

```bash
# Primary storage (required)
STORAGE_TARGET_1_NAME="local"
STORAGE_TARGET_1_TYPE="local"
STORAGE_TARGET_1_LOCAL_PATH="/mnt/backup/storage"

# Secondary storage (optional)
STORAGE_TARGET_2_NAME="nas"
STORAGE_TARGET_2_TYPE="sftp"
STORAGE_TARGET_2_SFTP_URL="192.168.1.100"
STORAGE_TARGET_2_SFTP_PORT="22"
STORAGE_TARGET_2_SFTP_USER="backup"
STORAGE_TARGET_2_SFTP_PATH="/volume1/backups"

# Tertiary storage (optional)
STORAGE_TARGET_3_NAME="backblaze"
STORAGE_TARGET_3_TYPE="b2"
STORAGE_TARGET_3_B2_BUCKETNAME="my-bucket"
STORAGE_TARGET_3_B2_ID="keyID"
STORAGE_TARGET_3_B2_KEY="applicationKey"

# Quarternary storage (optional)
STORAGE_TARGET_4_NAME="hetzner"
STORAGE_TARGET_4_TYPE="s3"
STORAGE_TARGET_4_S3_BUCKETNAME="my-bucket"
STORAGE_TARGET_4_S3_ENDPOINT="endpoint"
STORAGE_TARGET_4_S3_REGION="none"
STORAGE_TARGET_4_S3_ID="id"
STORAGE_TARGET_4_S3_SECRET="secret"
```

## Storage types

Every storage Duplicacy 3.2.5 supports is a type (ADR 23). Settings are plain `STORAGE_TARGET_N_<NAME>` variables; secrets are files (`/run/secrets/storage_target_n_<name>`, or `STORAGE_TARGET_N_<NAME>_FILE`). Settings in *italics* are optional; a `_PATH` places the storage in a folder inside its bucket, share or drive.

| Type | Settings | Secrets | Notes |
|---|---|---|---|
| `local` | `LOCAL_PATH` | | A path mounted into the container. |
| `sftp`, `sftpc` | `SFTP_URL`, `SFTP_PORT` (default 22), `SFTP_USER`, `SFTP_PATH` | the SSH keypair (see [Keys](configuring.md#configuration-sources)) | `sftpc` also offers older ciphers and key exchanges, for old servers. The path must exist. |
| `b2` | `B2_BUCKETNAME`, *`B2_PATH`* | `B2_ID`, `B2_KEY` | |
| `b2-custom` | `B2_DOWNLOAD_HOST`, `B2_BUCKETNAME`, *`B2_PATH`* | `B2_ID`, `B2_KEY` | Downloads through your own host (a CDN in front of B2). |
| `s3`, `s3c`, `minio`, `minios` | `S3_BUCKETNAME`, `S3_ENDPOINT`, *`S3_REGION`* (default `none`), *`S3_PATH`* | `S3_ID`, `S3_SECRET` | `s3c` is for providers that need V2 signatures; `minio` (HTTP) and `minios` (HTTPS) use path-style addressing for self-hosted servers. |
| `wasabi` | `WASABI_BUCKETNAME`, `WASABI_ENDPOINT` (default `s3.wasabisys.com`), `WASABI_REGION` (default `us-east-1`), *`WASABI_PATH`* | `WASABI_KEY`, `WASABI_SECRET` | |
| `azure` | `AZURE_ACCOUNT`, `AZURE_CONTAINER` | `AZURE_KEY` | The container must exist. |
| `gcs` | `GCS_BUCKETNAME`, *`GCS_PATH`* | `GCS_TOKEN`: a service-account JSON key | |
| `gcd` | `GCD_PATH` and/or *`GCD_DRIVE`* (a shared drive's ID, from its URL) | `GCD_TOKEN`: a service-account JSON key, or a token from your own OAuth app (below) | Google Drive. |
| `one` | `ONE_PATH`, `ONE_CLIENT_ID` | `ONE_TOKEN`, `ONE_CLIENT_SECRET` | OneDrive personal, through your own app registration (below). |
| `odb` | `ODB_PATH` and/or *`ODB_DRIVE_ID`*, `ODB_CLIENT_ID` | `ODB_TOKEN`, `ODB_CLIENT_SECRET` | OneDrive for Business or SharePoint, through your own app registration (below). |
| `dropbox` | `DROPBOX_PATH`, `DROPBOX_APP_KEY` | `DROPBOX_TOKEN` (a refresh token), `DROPBOX_APP_SECRET` | Through your own Dropbox app (below). |
| `swift` | `SWIFT_URL`: `user@auth-host/v3/container[/path][?domain=…&tenant=…]` | `SWIFT_KEY` | OpenStack Swift, in Duplicacy's URL form; add `protocol=http` for plain HTTP. |
| `webdav`, `webdav-http` | `WEBDAV_HOST` (host[:port]), `WEBDAV_USER`, `WEBDAV_PATH` | `WEBDAV_PASSWORD` | HTTPS or plain HTTP. The path must exist. |
| `smb` | `SMB_HOST` (host[:port]), `SMB_USER`, `SMB_SHARE`, *`SMB_PATH`* | `SMB_PASSWORD` | SMB 2/3. The path must exist. |
| `storj` | `STORJ_SATELLITE` (host:port), `STORJ_BUCKET`, *`STORJ_PATH`* | `STORJ_KEY`, `STORJ_PASSPHRASE` | |
| `fabric` | `FABRIC_ENDPOINT`, *`FABRIC_PATH`* | `FABRIC_TOKEN` | Storage Made Easy File Fabric. |

When Duplicacy cannot open a storage, the error says why: the storage's directory does not exist (Duplicacy does not create it on SFTP, WebDAV or SMB), or the storage cannot be reached, with the reason.

Checks default to daily on object storage and weekly on SFTP, WebDAV, SMB, File Fabric and the consumer drives (`STORAGE_TARGET_N_CHECK_INTERVAL` overrides). Every type receives the [recovery kit](recovery.md#automatic-recovery-kit), and the [envelope](recovery.md#break-glass-envelope) prints each type's settings and credentials.

**Your own app for Google Drive, OneDrive and Dropbox.** Archiver never refreshes tokens through duplicacy.com. Create an app with the provider (a Google Cloud OAuth client of type Desktop, a Microsoft Entra app registration with a client secret, or a Dropbox app), then get a token with the `rclone` in the image, on any machine with a browser (or with `rclone authorize` on your desktop):

```bash
docker run -it --rm --entrypoint rclone ghcr.io/sisyphusmd/archiver authorize dropbox "APP_KEY" "APP_SECRET"
docker run -it --rm --entrypoint rclone ghcr.io/sisyphusmd/archiver authorize onedrive "CLIENT_ID" "CLIENT_SECRET"
docker run -it --rm --entrypoint rclone ghcr.io/sisyphusmd/archiver authorize drive "CLIENT_ID" "CLIENT_SECRET"
```

Each prints a token as JSON. For Dropbox, `DROPBOX_TOKEN` is its `refresh_token`. For OneDrive, the whole JSON is `ONE_TOKEN` (or `ODB_TOKEN`). Microsoft replaces the token each time it is refreshed, so Duplicacy works from a copy under `logs/.tokens` and the recovery kit carries the latest one; the envelope cannot, and points to signing in to OneDrive instead. For Google Drive, `GCD_TOKEN` is `{"client_id": "CLIENT_ID", "client_secret": "CLIENT_SECRET", "end_point": {"TokenURL": "https://oauth2.googleapis.com/token"}, "token": <the JSON>}`; a service-account key needs none of this.

## Copies to secondary storages

The first target is the primary: every backup writes there. Each further target is a copy of it.

When the container runs on a schedule (`BACKUP_SCHEDULE`), each secondary has its own copy worker. A backup ends once the primary and the recovery kit are written, then wakes the workers, so a slow or unreachable offsite never delays or fails the next backup. Each worker copies every revision its target lacks, one copy at a time, with targets copying in parallel. A failed copy is retried after 1, 5 and 15 minutes, then every 30 minutes until the target has caught up. A target still failing after 30 minutes counts as down: one notification, a reminder every 24 hours while it stays down, and one when it recovers. Workers write to `logs/copies.log`, and `archiver status` shows each target's state. `archiver stop`, `pause` and `resume` act on the workers too; after a stop, a worker copies again after the next backup.

Without a schedule (manual mode or a one-shot `run backup`), there are no workers: the backup copies to every secondary itself and exits non-zero if a copy fails.

### Upload limits and copy windows

```bash
STORAGE_TARGET_1_UPLOAD_LIMIT="20000"        # kB/s for backups to the primary
STORAGE_TARGET_2_UPLOAD_LIMIT="2500"         # kB/s for copies to this secondary
STORAGE_TARGET_2_COPY_WINDOW="01:00-06:00"   # copy to it only between these local times
```

`STORAGE_TARGET_N_UPLOAD_LIMIT` caps how fast data is sent to that storage, in kilobytes per second: duplicacy's `-limit-rate` for a backup to the primary, `-upload-limit-rate` for a copy to a secondary. When several services back up at once (`BACKUP_PARALLELISM`), each gets an equal share of the primary's limit, so together they keep to it (the limit must be at least `BACKUP_PARALLELISM`); copies run one per secondary and get its whole limit.

`STORAGE_TARGET_N_COPY_WINDOW` holds a secondary's copy worker outside those hours (`TZ`, local time; `22:00-06:00` runs past midnight). Outside the window the worker copies, mirrors, prunes and checks nothing, the recovery kit is not uploaded there (it follows at a later run), and `archiver status` shows it waiting and when the window opens. A copy still running when the window closes ends there, and duplicacy picks up where it left off when the window next opens, without sending again what it already had. Backups to the primary are never held. Without the daemon, a backup outside a secondary's window skips that copy and says so; the next backup inside it copies everything since. A target with a window has a day longer to catch up before backup health counts it behind.

## Storage Outages

Before a backup, a copy, a check or a prune, archiver makes a quick read-only check that the storage can be reached with its credentials, so a storage that is down costs seconds rather than Duplicacy's long retries:

- **Primary down:** the backup fails at once. No service is started (no pre-backup hook stops anything for a backup that cannot happen), each is reported skipped, and one **PRIMARY DOWN** notification is sent at a higher priority (Pushover priority 1, ntfy 5); backup health shows FAILING. If the primary is lost mid-run, a failed service backup checks it again, and when it is gone no further service starts (post-backup hooks still run for services whose pre hook ran). The next backup that reaches it sends one recovery notice.
- **Secondary down:** its copy is skipped with one line. The copy workers retry on their schedule, each retry checking first; a storage still down after 30 minutes is one alert.
- **During maintenance:** a secondary that cannot be reached fails its own check and prune, and the others are still maintained. The secondaries are registered through the primary, so a primary that cannot be reached fails the maintenance run.
