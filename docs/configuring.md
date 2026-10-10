# Configuring Archiver

What to back up, how secrets are supplied, and how notifications reach you.

The settings below define what to backup and where. Supply them as environment variables plus file-based secrets (see [Configuration Sources](#configuration-sources) below), and edit them wherever they live — your compose file, ConfigMap, or secret store.

**Every setting is listed in the [configuration reference](configuration.md)**, generated from the code, so it is never out of date: each variable's default, accepted values and meaning, and each storage type's variables. [docs/archiver.schema.json](archiver.schema.json) is the same as a JSON Schema, for an editor or a tool such as `check-jsonschema` to complete and check a set of settings; it also flags a secret set as an environment variable instead of a file.

## Configuration Sources

Environment variables carry the non-secret settings and files under `/run/secrets` carry the secrets and keys, so the configuration stays under version control (compose file / ConfigMap) and the secrets stay in a secret store. Nothing is read from a configuration file, and nothing configured is ever executed.

**Non-secret settings (plain env vars).** Every setting the [configuration reference](configuration.md) does not mark as a secret: `SERVICE_DIRECTORIES`, the schedules, each target's `STORAGE_TARGET_N_*` settings (`NAME`, `TYPE`, and its type's settings in [Storage types](storage.md#storage-types)), and those for maintenance, drills, notifications and monitoring. As an env var, `SERVICE_DIRECTORIES` is a colon-delimited list rather than a bash array, for example `SERVICE_DIRECTORIES=/srv/*/:/home/user/data/` (newlines also work, so a YAML block scalar is fine).

**Secrets (files only).** Secrets are never read from a plain env var (one would leak through `/proc` and `docker inspect`, and Archiver purges any it finds). Each secret is read from a file: `<NAME>_FILE` if set, otherwise `/run/secrets/<lowercased name>`. The secrets are those the reference marks as one: `STORAGE_PASSWORD`, `RSA_PASSPHRASE`, `RECOVERY_PASSWORD`, the notification credentials (`PUSHOVER_USER_KEY`, `PUSHOVER_API_TOKEN`, `APPRISE_URL`, `NTFY_TOKEN`), and each target's type's secrets (see [Storage types](storage.md#storage-types)) and optional [break-glass credentials](recovery.md#break-glass-envelope). For example, `STORAGE_PASSWORD` reads `/run/secrets/storage_password` and `STORAGE_TARGET_1_B2_KEY` reads `/run/secrets/storage_target_1_b2_key`. `STORAGE_PASSWORD` must be at least 8 characters (a Duplicacy requirement). Because `/run/secrets` is the native mount path for Docker and Kubernetes secrets, a Compose or Swarm `secrets:` entry named to match (for example `storage_password`) is picked up with no extra configuration.

**Keys (files).** Keys are always files under `/opt/archiver/keys`. The RSA keypair must be provided as files at `/run/secrets/rsa_private_key` and `/run/secrets/rsa_public_key` (override the paths with `RSA_PRIVATE_KEY_FILE` / `RSA_PUBLIC_KEY_FILE`). The SFTP keypair is optional, for sftp targets, at `/run/secrets/ssh_private_key` and `/run/secrets/ssh_public_key` (override with `SSH_PRIVATE_KEY_FILE` / `SSH_PUBLIC_KEY_FILE`; restore needs both halves).

Starting from scratch without `archiver init`? Generate the RSA keypair yourself — Duplicacy needs the traditional PKCS#1 PEM format, and the passphrase must match your `rsa_passphrase` secret:

```bash
openssl genrsa -aes256 -passout pass:YOUR_RSA_PASSPHRASE -traditional -out rsa_private_key 2048
openssl rsa -in rsa_private_key -passin pass:YOUR_RSA_PASSPHRASE -pubout -out rsa_public_key
```

(For sftp targets, also `ssh-keygen -t ed25519 -N "" -f ssh_private_key`, which writes `ssh_private_key` and `ssh_private_key.pub` — supply the latter as `ssh_public_key`.)

**What you must not lose (disaster recovery).** To restore after losing the host you need, stored somewhere that does not burn down with it: `STORAGE_PASSWORD` (unlocks the Duplicacy storage), `RSA_PASSPHRASE` + `rsa_private_key` (decrypt the file data), your storage-target settings (`archiver.env` or equivalents), and for sftp targets the SSH keypair. Missing any of the first three means the backups are permanently undecryptable. The [Automatic Recovery Kit](recovery.md#automatic-recovery-kit) keeps all of it on every storage target for you — one password in your password manager covers everything.

## Service Directories

Directories to backup, colon-delimited. Use `*` for subdirectories:

```bash
SERVICE_DIRECTORIES=/srv/*/:/home/user/data/
# /srv/*/           -> each subdirectory becomes its own repository
# /home/user/data/  -> a single repository
```

(Newlines work as separators too, so a YAML block scalar is fine.)

Each directory's name becomes part of its snapshot ID (`<hostname>-<name>`), which Duplicacy restricts to letters, digits, `_` and `-`: a directory named with a space or a dot cannot be backed up, so rename it. An entry that matches no directory (a typo or an unmounted volume) is reported as an error on every backup, and so is a directory whose name breaks that rule; the other directories still back up.

## Performance

```bash
DUPLICACY_THREADS="10"
BACKUP_PARALLELISM="2"
```

`DUPLICACY_THREADS` is the number of parallel upload/download threads for each duplicacy operation. (Default: 4)

`BACKUP_PARALLELISM` is how many services a backup processes at once, each with its pre-backup hook, backup and post-backup hook. (Default: 2.) Set `1` to back them up one after another, in order, as before v1, for example when one service's hook depends on another's having finished. Each running service uses `DUPLICACY_THREADS` threads, so peak load grows with both.

## Notifications

Archiver sends to any combination of Pushover, an [Apprise API](https://github.com/caronc/apprise-api) server and [ntfy](https://ntfy.sh). Each destination receives the kinds of notification its setting asks for:

| `NOTIFY_ON` | Sent |
|---|---|
| `failures` (default) | A backup, copy, restore or drill that failed; a refused backup; a storage down or a failed storage check, and its recovery |
| `problems` | Also warning signs, such as a mirror pass refused for deleting too much |
| `everything` | Also routine news: backup and maintenance completed, a run paused, resumed or stopped |

`NOTIFY_ON` sets every destination; `PUSHOVER_NOTIFY_ON`, `APPRISE_NOTIFY_ON` and `NTFY_NOTIFY_ON` override it for one (for example `failures` to your phone, `everything` to ntfy). Log levels (INFO, WARNING, ERROR) are about log lines and do not decide notifications.

A notification is one per incident, not one per error line: a failed backup sends one message listing its errors (the first ten; the log has them all) when it ends, a storage down is one alert, not one per retry. While an incident lasts it is notified again every `ALERT_REPEAT_INTERVAL` (default `24h`; `6h`, `2d`, or `0` for never), and when it clears (a clean backup, a storage caught up, a check or drill passing again) one recovery notice goes to the destinations the alert went to. The open incidents are kept in `logs/.incidents.json`. `archiver notify test` sends one message to every destination, saying which events it receives.

```bash
# Pushover
NOTIFICATION_SERVICE="Pushover"   # with secrets pushover_user_key and pushover_api_token

# Apprise API: the notify URL (with user:password@ if the server needs basic auth) is the
# secret apprise_url; APPRISE_TAGS optionally sends each kind with an Apprise tag
APPRISE_TAGS="failure=critical,problem=alerts,routine=quiet"

# ntfy: server and topic; an access token, if the topic needs one, is the secret ntfy_token
NTFY_URL="https://ntfy.sh/my-archiver"
```

Apprise receives `title`, `body`, a `type` (info, warning, failure) and the kind's tag when `APPRISE_TAGS` names one (otherwise `all`, so every URL of the configuration is notified, tagged or not); an HTTP 424 (some URL of the tag failed) is logged as a failed notification and not retried, so the URLs that did receive it are not sent it twice. ntfy gets priority 2 for routine news, 3 for problems and 4 for failures.

### Setting up Pushover

1. [Create account](https://pushover.net/signup) or [sign in](https://pushover.net/login)
2. Note your **User Key** from the dashboard
3. [Add a device](https://pushover.net/clients) to receive notifications
4. [Create an Application/API Token](https://pushover.net/apps/build)
5. Name your app and agree to terms
6. Save the **API Token/Key**

You'll enter the **User Key** and **API Token** during init.
