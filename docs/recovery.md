# Disaster Recovery

Your configuration and secrets are the keys to every backup. The recovery kit keeps them on every storage, the envelope makes sure you can reach one, and `archiver recover` rebuilds a lost host from them.

## Automatic Recovery Kit

Your configuration and secrets are the keys to every backup: lose them and the backups are unreadable. The recovery kit automates keeping them safe. It activates when the `recovery_password` secret is present (at `/run/secrets/recovery_password`, or via `RECOVERY_PASSWORD_FILE`) — the shipped compose template includes it.

**Where the password comes from.** `archiver init` generates it, includes it in the emitted secrets, and displays it once — **save it in your password manager**; it is the one credential you personally keep. An existing deployment enables the kit with one command plus a `secrets:` entry:

```bash
openssl rand -base64 24 > secrets/recovery_password    # save a copy in your password manager
```

It must be at least 8 characters and **differ from `STORAGE_PASSWORD`** (it is the only thing protecting the kit at rest, and the kit contains the storage password); archiver refuses a violating configuration. To rotate it, replace the secret file and update your password manager — the next backup re-encrypts and re-uploads everywhere automatically.

**What it does.** After each backup, archiver assembles the kit — the full effective configuration (every non-secret setting, every secret, and the keys), generated recreation notes (`RECREATE.txt`: hostname, schedule, every container path that needs a mount, required capabilities), and your deployment manifests if mounted (below) — encrypts it to that password (AES-256, pbkdf2), and uploads it as a **plain file beside the duplicacy data on every storage target**: `archiver-recovery-kit-<hostname>.tar.enc`, with a companion `README.txt` holding the decrypt command. It is not inside duplicacy's storage format, so you can download it from any provider web UI or file browser with no tooling. The kit is re-encrypted and re-uploaded only when its content actually changes (or a new storage target appears); an unchanged config is a nightly no-op. (The name includes the hostname, so deployments sharing a storage target must have distinct hostnames — they already need that for distinct snapshot IDs.)

**Capturing your deployment manifest.** The kit cannot see files outside the container, so it cannot magically include your compose file or Kubernetes YAML — instead, anything mounted (read-only) at `/opt/archiver/deployment/` is captured **verbatim** into the kit. Each runtime uses its native mechanism:

- **Docker/Podman Compose**: `- ./compose.yaml:/opt/archiver/deployment/compose.yaml:ro` (in the shipped template — the manifest captures itself).
- **Kubernetes (incl. Flux/Argo GitOps)**: put the manifests in a ConfigMap and mount it at `/opt/archiver/deployment`. With kustomize, a `configMapGenerator` entry (`files: [archiver.yaml]`) keeps the ConfigMap — and therefore the kit — current on every git change automatically.
- **NixOS / systemd-managed containers**: bind the service definition, e.g. `"/etc/nixos/services/archiver.nix:/opt/archiver/deployment/archiver.nix:ro"`.

In GitOps setups your manifest is already replicated in git; the kit's copy is for the total-loss case where the git host is gone too. Without a mounted manifest the kit still stands alone: `RECREATE.txt` lists every fact archiver knows about how the container must be put together, and the compose template covers the rest.

**Carrying more in the kit.** `RECOVERY_KIT_EXTRA_PATHS` lists paths in the container (colon- or newline-separated; mount them read-only) that go into the kit's `extra/`: a disaster-recovery runbook, the scripts your restore hooks call, `git bundle` copies of the repositories you rebuild from. A recovery then has them before anything else is restored. A path that does not exist is reported as an error, and the kit goes out without it. Everything here is re-encrypted and re-uploaded whenever it changes, so keep large or fast-changing files out.

**Recovery** happens on any machine with stock `openssl` — no archiver, no other files, just the recovery password from your password manager: reach **any one** of your storage locations, download the kit, and run

```bash
openssl enc -d -aes-256-cbc -pbkdf2 -in archiver-recovery-kit-<hostname>.tar.enc | tar -xvf -
```

It prompts for the password and yields `archiver.env` + `secrets/` + `RECREATE.txt` (+ `deployment/` with your manifests) — everything needed to recreate the deployment (the recovery password itself is included, so the recreated deployment maintains its kit immediately). From there, restore your data with `archiver restore` as usual.

`archiver recovery-kit` uploads on demand (useful right after setup); `archiver recovery-kit force` re-uploads everywhere even if unchanged. An upload failure is logged and notified but never fails the backup; the failed target is retried on the next run. The kit is **write-only**: nothing ever reads it back at runtime, so it is never a boot dependency.

On B2, each update creates a new file version; old versions age out per your bucket lifecycle rules (they are ciphertext, so lingering versions are harmless).

### Break-glass envelope

The kit needs two things to be useful: the recovery password, and a way to reach one storage. If both live only in systems the backups protect (a password manager hosted on the same servers, a login whose 2FA is there), recovery is circular. The envelope breaks the circle: one printed page, kept somewhere that does not share fate with the backups, from which any one storage is enough to recover with no help from anyone.

```bash
docker exec archiver archiver envelope              # writes /opt/archiver/envelope/envelope-<hostname>.{pdf,html}
docker cp archiver:/opt/archiver/envelope ./envelope && docker exec archiver rm -rf /opt/archiver/envelope
# print ./envelope/envelope-<hostname>.pdf, then:
docker exec archiver archiver envelope confirm
rm -rf ./envelope
```

The page holds the recovery password, the decrypt command, and for each storage target where the kit sits (address, user, bucket, path) with a credential that can read it, as text and as QR codes, plus space to write account-recovery codes by hand. Each storage's block also has a command that downloads the kit from any machine: `sftp` for an SFTP storage, and for the others one `rclone copyto` line that needs nothing but rclone (no config file; it carries the credential shown in `RCLONE_CONFIG_KIT_*` variables, so it never appears in the process list). A OneDrive token changes as it is used, so a OneDrive block says to sign in and download the kit instead. The files are plaintext, owner-only, and never sent anywhere: print one and delete them.

**Break-glass credentials.** By default each storage's block carries its backup credential, marked **FULL ACCESS**, because whoever holds the page could also delete those backups. Where the provider allows a narrower credential, create one that can only read the bucket and give it to archiver as a secret file; the page then carries it instead (archiver never uses it for anything else):

| Storage type | Secret files (`/run/secrets/...`) |
|---|---|
| B2 (`b2`, `b2-custom`) | `storage_target_N_breakglass_b2_id`, `storage_target_N_breakglass_b2_key` (a key with `listBuckets`, `listFiles`, `readFiles` on the bucket) |
| S3 (`s3`, `s3c`, `minio`, `minios`) | `storage_target_N_breakglass_s3_id`, `storage_target_N_breakglass_s3_secret` |
| SFTP (`sftp`, `sftpc`) | `storage_target_N_breakglass_ssh_key` (a private key for a read-only account, whose name goes in the `STORAGE_TARGET_N_BREAKGLASS_SFTP_USER` env var; otherwise the page carries the backup key, since the kit holding it is itself on that server) |
| Any other type | `storage_target_N_breakglass_<secret>` for each of its keys, passwords and tokens that has one: `wasabi_key`/`wasabi_secret`, `azure_key`, `swift_key`, `webdav_password`, `smb_password`, `storj_key`, `fabric_token` (the S3 and B2 variants use the rows above). The page uses them only when all of a target's are set. Drive tokens, app secrets, service-account keys and the Storj passphrase have none: the page carries the backup ones, marked **FULL ACCESS**. |

**Keeping it current.** `archiver envelope confirm` records a fingerprint of what the page says (a hash keyed by the recovery password; it reveals nothing). After every recovery-kit run archiver compares it with what the page would say now: when a secret or storage on it changes, `archiver status` shows `Envelope: OUT OF DATE`, healthcheck warns, and one notification is sent. A year after confirming, the same happens as a reminder to check the envelope is still there and readable.

## Recovering a Lost Host (`archiver recover`)

When the host is gone, all you need is the recovery kit (on every storage, and fetched with the envelope's command) and its password. On the new host, start a one-off container with the service directories mounted where the old deployment had them, the kit, and an empty directory for the recovered configuration:

```bash
docker run -it --rm --hostname <old hostname> \
  -v ./archiver-recovered:/opt/archiver/recovered \
  -v ./archiver-recovery-kit-<old hostname>.tar.enc:/kit.tar.enc:ro \
  -v /srv:/srv \
  forgejo.bryantserver.com/sisyphusmd/archiver:1 recover /kit.tar.enc
```

It asks for the kit password (or reads a mounted `recovery_password` secret), then, checking each step before the next: decrypts the kit, writes its `archiver.env`, `secrets/` (owner-only) and `RECREATE.txt` to `/opt/archiver/recovered`, puts the keys in place, checks every storage, and, after you confirm (`--yes` skips the question), restores every service of the old host into its directory from `SERVICE_DIRECTORIES` (a pattern like `/srv/*/` places each service beside its siblings, even though the directories do not exist yet). `--hook` also runs each service's restore hook. It ends by saying how to recreate the deployment: `archiver.env` as its environment, `secrets/` as `/run/secrets`, and the old hostname, so new backups continue the same snapshots. Move the plaintext secrets into your secret store and delete the directory afterwards.
