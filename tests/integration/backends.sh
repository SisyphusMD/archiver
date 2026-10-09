#!/usr/bin/env bash
# Round trips through every storage type an emulator can stand in for (ADRs 23, 25): each is
# the primary of a fresh archiver container, which backs up, places the recovery kit, and
# restores. The kit is fetched back through a remote written here, not by archiver's code,
# and decrypted with plain openssl. Types only a real service provides run in the real-service job (ADRs 25, 27).
#
# One TLS front (Caddy, a throwaway CA the archiver containers trust) serves the HTTPS
# names: Garage for minios/s3c/wasabi (virtual-host style needs the bucket alias), Azurite
# behind Azure's fixed *.blob.core.windows.net, and WebDAV over HTTPS.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/backends.sh [type ...]

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
P="archiver-be-$$"
NET="$P-net"
CFG="$P-cfg"
# Digest-pinned: this test gates releases. (Renovate does not scan shell scripts; bump the
# digests by hand.) Garage and Caddy match s3-garage.sh, atmoz/sftp matches sftp-runtime.sh.
GARAGE_IMAGE="dxflrs/garage:v2.3.0@sha256:866bd13ed2038ba7e7190e840482bc27234c4afaf77be8cfa439ae088c1e4690"
CADDY_IMAGE="caddy:2.11.4-alpine@sha256:6aeddd44c3078b0f9a35206472a11420648a79c184603ef95957d0a20044cb2b"
SFTP_IMAGE="atmoz/sftp:alpine@sha256:a6cb3eb29202ca7f57e73bb7e527286e66e0e822fff65609207c7e0ef2d135a3"
SAMBA_IMAGE="ghcr.io/servercontainers/samba:latest@sha256:ac7c406702a3bf4137fcd04ce031d87adbda1b819f698fe89a89b5ee46618dfb"
SWIFT_IMAGE="openstackswift/saio:latest@sha256:3bfb86f881faafa0994be1901f1a0a9a182d976d41cbceb18fbecfe017d0fc66"
# SeaweedFS answers V2-signed requests, which s3c (goamz) sends and Garage refuses.
SEAWEED_IMAGE="chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d"
FAKEGCS_IMAGE="fsouza/fake-gcs-server:1.56.1@sha256:797ce226d62f947c009dc40246b30cfb456b8473d8241407f9d6f2c04e4d69ef"
AZURITE_IMAGE="mcr.microsoft.com/azure-storage/azurite:latest@sha256:830430c1da1a2d537e08f3e6764dd1f5ae00cf0346bcaf625b968ec3f0971fd5"

S3_KEY="GK0123456789abcdef01234567"
S3_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
BUCKET="archiver-test"
REGION="garage"
# An account of the test's own: the Azure SDK in Duplicacy sends Azurite's well-known
# devstoreaccount1 to 127.0.0.1:10000 instead of <account>.blob.core.windows.net.
AZ_ACCOUNT="archivertest"
AZ_KEY="YXJjaGl2ZXItdGVzdC1henVyZS1rZXktMDEyMzQ1Njc4OWFiY2RlZg=="
AZ_HOST="$AZ_ACCOUNT.blob.core.windows.net"
RECOVERY="recovery-pw"

ALL=(minio minios s3c wasabi sftpc webdav webdav-http smb swift azure gcs dropbox)
WANT=("$@")
[ ${#WANT[@]} -gt 0 ] || WANT=("${ALL[@]}")
wanted() { local t; for t in "${WANT[@]}"; do [ "$t" = "$1" ] && return 0; done; return 1; }

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
# fail NAME MESSAGE: die with the end of that archiver container's log, Duplicacy's own words.
fail() { docker exec "$P-$1" bash -c 'tail -n 40 /opt/archiver/logs/archiver.log' >&2 2>/dev/null; die "$1: $2"; }
cleanup() {
  docker ps -aq --filter "name=^$P-" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$CFG" >/dev/null 2>&1 || true
}
trap cleanup EXIT
# KEEP=1 leaves the containers, network and volume behind for debugging.
[ -n "${KEEP:-}" ] && trap - EXIT
garage() { docker exec "$P-garage" /garage -c /cfg/garage.toml "$@"; }

docker network create "$NET" >/dev/null
docker volume create "$CFG" >/dev/null

log "a throwaway CA's cert for every HTTPS name, Garage's config, the Caddyfile, an SSH key"
docker run --rm -i -v "$CFG":/cfg --entrypoint bash "$IMAGE" -s <<EOF || die "config generation failed"
set -e
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -keyout /cfg/private.key -out /cfg/public.crt -subj '/CN=archiver-test' \
  -addext 'subjectAltName=DNS:garage,DNS:${BUCKET}.garage,DNS:${AZ_HOST},DNS:webdav-tls,DNS:seaweed,DNS:api.dropboxapi.com,DNS:content.dropboxapi.com,DNS:www.googleapis.com,DNS:storage.googleapis.com,DNS:oauth2.googleapis.com' 2>/dev/null
chmod 644 /cfg/public.crt /cfg/private.key
ssh-keygen -t ed25519 -N "" -f /cfg/id_ed25519 -q
mkdir -p /cfg/sshkeys && cp /cfg/id_ed25519.pub /cfg/sshkeys/ && chmod 644 /cfg/id_ed25519 /cfg/sshkeys/id_ed25519.pub
cat > /cfg/garage.toml <<TOML
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "\$(openssl rand -hex 32)"

[s3_api]
s3_region = "${REGION}"
api_bind_addr = "[::]:3900"
root_domain = ".garage"
TOML
# Request logs keep the Authorization header: the Dropbox case reads which token was used.
cat > /cfg/Caddyfile <<CADDY
{
	auto_https off
	admin off
	servers {
		log_credentials
	}
}
# Dropbox stand-in: the token endpoint answers, every API call is refused as not found.
https://api.dropboxapi.com, https://content.dropboxapi.com {
	tls /cfg/public.crt /cfg/private.key
	log {
		output stdout
		format json
	}
	header Content-Type application/json
	handle /oauth2/token {
		respond "{\\"access_token\\":\\"fake-access\\",\\"token_type\\":\\"bearer\\",\\"expires_in\\":14400}" 200
	}
	handle {
		respond "{\\"error_summary\\":\\"path/not_found/..\\",\\"error\\":{\\".tag\\":\\"path\\",\\"path\\":{\\".tag\\":\\"not_found\\"}}}" 409
	}
}
https://garage, https://${BUCKET}.garage {
	tls /cfg/public.crt /cfg/private.key
	reverse_proxy garage-api:3900
}
https://${AZ_HOST} {
	tls /cfg/public.crt /cfg/private.key
	reverse_proxy azurite:10000
}
https://webdav-tls {
	tls /cfg/public.crt /cfg/private.key
	reverse_proxy webdav-srv:8080
}
# Google Cloud Storage stand-in: fake-gcs-server behind Google's API hosts, and a token
# endpoint that grants any signed service-account assertion.
https://www.googleapis.com, https://storage.googleapis.com {
	tls /cfg/public.crt /cfg/private.key
	# fake-gcs omits it on object downloads; Duplicacy's GCS library parses it beside X-Goog-Generation.
	header ?X-Goog-Metageneration 1
	reverse_proxy fake-gcs:4443
}
https://oauth2.googleapis.com {
	tls /cfg/public.crt /cfg/private.key
	header Content-Type application/json
	respond "{\\"access_token\\":\\"gcs-access\\",\\"token_type\\":\\"Bearer\\",\\"expires_in\\":3600}" 200
}
https://seaweed {
	tls /cfg/public.crt /cfg/private.key
	reverse_proxy seaweed-s3:8333
}
CADDY
cat > /cfg/seaweed.json <<JSON
{"identities": [{"name": "archiver", "credentials": [{"accessKey": "${S3_KEY}", "secretKey": "${S3_SECRET}"}],
  "actions": ["Admin", "Read", "Write", "List", "Tagging"]}]}
JSON
EOF

docker run -d --name "$P-proxy" --network "$NET" --network-alias garage --network-alias "$BUCKET.garage" \
  --network-alias "$AZ_HOST" --network-alias webdav-tls --network-alias seaweed \
  --network-alias api.dropboxapi.com --network-alias content.dropboxapi.com \
  --network-alias www.googleapis.com --network-alias storage.googleapis.com --network-alias oauth2.googleapis.com \
  -v "$CFG":/cfg "$CADDY_IMAGE" caddy run --config /cfg/Caddyfile --adapter caddyfile >/dev/null || die "caddy failed to start"
# The real hosts answer for any alias the front does not hold, so a front that failed to start
# must stop the run rather than let a case reach Google, Azure or Dropbox.
sleep 2
[ "$(docker inspect -f '{{.State.Running}}' "$P-proxy" 2>/dev/null)" = true ] || { docker logs "$P-proxy" >&2; die "the TLS front is not running"; }

# start_archiver NAME [docker run args...]: an archiver container on the test network with the
# RSA keys, the common secrets, the fixtures, and the test CA trusted.
start_archiver() {
  local name="$1"; shift
  docker run -d --name "$P-$name" --hostname "be-$name" --network "$NET" --cap-drop ALL --cap-add DAC_OVERRIDE \
    -v "$CFG":/cfg:ro -e SERVICE_DIRECTORIES=/data/fixtures/ "$@" --entrypoint bash "$IMAGE" -c 'sleep 1800' >/dev/null \
    || die "$name: archiver container failed to start"
  docker exec "$P-$name" bash -c "
    set -e
    mkdir -p /opt/archiver/keys /run/secrets /data/fixtures
    openssl genrsa -aes256 -passout pass:testpassphrase -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
    openssl rsa -in /opt/archiver/keys/private.pem -passin pass:testpassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
    chmod 600 /opt/archiver/keys/private.pem
    printf testpassword > /run/secrets/storage_password
    printf testpassphrase > /run/secrets/rsa_passphrase
    printf '$RECOVERY' > /run/secrets/recovery_password
    echo 'hello via $name' > /data/fixtures/file.txt
    head -c 70000 /dev/urandom > /data/fixtures/blob.bin
    cp /cfg/public.crt /usr/local/share/ca-certificates/archiver-test.crt && update-ca-certificates >/dev/null
  " || die "$name: in-container setup failed"
}

# secret NAME FILE VALUE: a secret file, written from stdin (never argv or the environment).
secret() { printf '%s' "$3" | docker exec -i "$P-$1" sh -c "cat > /run/secrets/$2"; }

# until_in NAME SECONDS COMMAND: wait for COMMAND to succeed inside that container.
until_in() {
  local name="$1" secs="$2"; shift 2
  for _ in $(seq 1 "$secs"); do docker exec "$P-$name" bash -c "$*" >/dev/null 2>&1 && return 0; sleep 1; done
  return 1
}

# The type under test, set by each case: its STORAGE_TARGET_N_ settings as FIELD=value, its
# secret files as FIELD=value, and a PREP function run in each container (NAME N). %D stands
# for the storage's directory, so the primary and copy phases each start a storage of their own.
FIELDS=() SECRETS=() PREP=""
target_args() { # N DIR
  local f; TARGS=()
  for f in "${FIELDS[@]}"; do TARGS+=(-e "STORAGE_TARGET_$1_${f//%D/$2}"); done
}
put_secrets() { # NAME N
  local kv
  for kv in "${SECRETS[@]}"; do
    secret "$1" "storage_target_$2_$(printf '%s' "${kv%%=*}" | tr '[:upper:]' '[:lower:]')" "${kv#*=}" || die "$1: secret not written"
  done
}

# run_case TYPE KIT-DIR [RCLONE_CONFIG_CHECK_* settings...]: the type as a primary (backup, the
# kit fetched independently from KIT-DIR and decrypted, restore), then as a copy of a local
# primary through everything a secondary goes through.
run_case() {
  local name="$1" dir="$2"; shift 2
  target_args 1 store
  start_archiver "$name" -e STORAGE_TARGET_1_NAME=offsite -e ROTATE_BACKUPS=false "${TARGS[@]}"
  put_secrets "$name" 1
  [ -z "$PREP" ] || "$PREP" "$name" 1 || die "$name: setup failed"
  roundtrip "$name" "${dir//%D/store}" "$@"
  copy_lifecycle "$name"
}

# roundtrip NAME DIR [RCLONE_CONFIG_CHECK_*=... ...]: back up, fetch the kit from DIR on the
# remote described by the rest and decrypt it, then restore and compare.
roundtrip() {
  local name="$1" dir="$2"; shift 2
  local envs=() kv
  for kv in "$@"; do envs+=(-e "RCLONE_CONFIG_CHECK_$kv"); done
  log "$name: backup"
  docker exec "$P-$name" archiver backup || fail "$name" "backup exited non-zero"
  log "$name: the kit, fetched independently and decrypted with only the password"
  docker exec "${envs[@]}" "$P-$name" bash -c '
    set -e
    rclone --config /dev/null cat "CHECK:$1/archiver-recovery-kit-be-$2.tar.enc" > /tmp/kit.enc
    openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:$3" -in /tmp/kit.enc | tar -xf - -C /tmp
    grep -q "^STORAGE_TARGET_1_TYPE=$2\$" /tmp/archiver.env
  ' _ "$dir" "$name" "$RECOVERY" || die "$name: recovery kit missing from the storage or not decryptable"
  log "$name: the envelope's fetch command, run as printed, downloads the kit"
  docker exec "$P-$name" bash -c '
    set -e
    archiver envelope /tmp/env >/dev/null
    cmd=$(grep -o "<pre>RCLONE_CONFIG_KIT_TYPE=.*</pre>" /tmp/env/envelope-*.html | head -n 1 | sed -e "s#^<pre>##" -e "s#</pre>\$##" \
      -e "s/&quot;/\"/g" -e "s/&lt;/</g" -e "s/&gt;/>/g" -e "s/&amp;/\&/g")
    mkdir /tmp/fetch && cd /tmp/fetch
    if [ -z "$cmd" ]; then
      # An SFTP page prints an sftp command and the key to save as ssh_private_key.
      cmd=$(grep -o "<pre>Fetch: sftp .*</pre>" /tmp/env/envelope-*.html | head -n 1 | sed -e "s#^<pre>Fetch: ##" -e "s#</pre>\$##")
      [ -n "$cmd" ]
      install -m 600 /opt/archiver/keys/id_ed25519 ssh_private_key
    fi
    bash -c "$cmd" >/dev/null
    openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:$1" -in "archiver-recovery-kit-be-$2.tar.enc" | tar -tf - | grep -q archiver.env
  ' _ "$RECOVERY" "$name" || die "$name: the envelope's fetch command did not download a decryptable kit"
  log "$name: restore"
  docker exec -e SNAPSHOT_ID="be-$name-fixtures" -e LOCAL_DIR=/data/restore -e OVERWRITE=1 -e HASH_COMPARE=1 -e IGNORE_OWNERSHIP=1 \
    "$P-$name" archiver auto-restore || fail "$name" "auto-restore exited non-zero"
  docker exec "$P-$name" bash -c 'diff /data/fixtures/file.txt /data/restore/file.txt && cmp /data/fixtures/blob.bin /data/restore/blob.bin' \
    || die "$name: restored content differs from the source"
  docker rm -f "$P-$name" >/dev/null
  echo "=== $name OK as the primary ==="
}

# copy_lifecycle TYPE: the type as the copy of a local primary: an inline copy, maintenance's
# check and prune, then under the daemon its copy worker's hand-off, check, mirroring of a
# revision local pruned, exhaustive prune, pause, resume and stop, and a restore from it.
copy_lifecycle() {
  local name="$1-copy" id="be-$1-copy-fixtures" lg=/opt/archiver/logs
  target_args 2 copy
  start_archiver "$name" -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/store \
    -e STORAGE_TARGET_2_NAME=offsite "${TARGS[@]}" -e PRUNE_BACKUPS=true
  put_secrets "$name" 2
  [ -z "$PREP" ] || "$PREP" "$name" 2 || die "$name: setup failed"

  log "$name: a backup copies to it inline"
  docker exec "$P-$name" archiver backup || fail "$name" "backup with an inline copy exited non-zero"
  docker exec "$P-$name" grep -q "Copy to offsite storage completed" "$lg/archiver.log" || fail "$name" "no inline copy"
  log "$name: maintenance checks and prunes it"
  docker exec "$P-$name" archiver maintenance >/dev/null || { docker exec "$P-$name" tail -n 30 "$lg/maintenance.log" >&2; die "$name: maintenance exited non-zero"; }
  docker exec "$P-$name" grep -q "Storage check completed for offsite" "$lg/maintenance.log" || die "$name: maintenance did not check it"

  log "$name: its copy worker takes a backup's copy and checks it"
  docker exec -d "$P-$name" sh -c 'BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/daemon.out 2>&1'
  until_in "$name" 60 'archiver status | grep -q "offsite: caught up"' || fail "$name" "worker never caught up"
  docker exec "$P-$name" bash -c 'sleep 1.1; echo two > /data/fixtures/file.txt; archiver backup >/dev/null 2>&1' || fail "$name" "second backup failed"
  docker exec "$P-$name" grep -q "run in the background" "$lg/archiver.log" || fail "$name" "backup did not hand its copy to the worker"
  until_in "$name" 120 "grep -q 'Copy to offsite storage completed' $lg/copies.log" || { docker exec "$P-$name" tail -n 30 "$lg/copies.log" >&2; die "$name: worker did not copy"; }
  until_in "$name" 120 "grep -q 'Check of offsite storage completed' $lg/copies.log" || { docker exec "$P-$name" tail -n 30 "$lg/copies.log" >&2; die "$name: worker did not check"; }

  if [ -n "${NO_MOVE:-}" ]; then
    # Deleting a revision fossilizes chunks with the storage's move, which this one's emulator lacks.
    echo "--- $name: mirroring and exhaustive prune not run here ($NO_MOVE); the real-service job covers them (ADR 27)"
  else
    log "$name: a revision local prunes is mirrored off it"
    docker exec -w /data/fixtures -e DUPLICACY_LOCAL_PASSWORD=testpassword -e DUPLICACY_LOCAL_RSA_PASSPHRASE=testpassphrase "$P-$name" \
      duplicacy prune -storage local -id "$id" -r 1 >/dev/null || die "$name: local prune of revision 1 failed"
    docker exec "$P-$name" bash -c "archiver mirror --dry-run | grep -q 'offsite: delete $id revisions 1\$'" || die "$name: mirroring does not plan the deletion"
    docker exec "$P-$name" archiver daemon ctl local-changed >/dev/null || die "$name: could not wake the worker"
    until_in "$name" 120 "grep -q 'Mirror: deleting revisions 1 of $id from offsite storage' $lg/copies.log" || die "$name: revision 1 not mirrored"
    until_in "$name" 60 "! archiver mirror --dry-run | grep -q 'offsite: delete'" || die "$name: revision 1 still on it after mirroring"
  
    log "$name: exhaustive prune, pause, resume and stop reach its worker"
    # The worker may have pruned exhaustively while catching up; wait for the one asked for here.
    local before
    before=$(docker exec "$P-$name" grep -c 'Exhaustive prune of offsite storage completed' "$lg/copies.log")
    docker exec "$P-$name" archiver maintenance exhaustive >/dev/null || die "$name: exhaustive maintenance failed"
    until_in "$name" 120 "[ \$(grep -c 'Exhaustive prune of offsite storage completed' $lg/copies.log) -gt $before ]" || die "$name: worker did not prune exhaustively"
  fi
  docker exec "$P-$name" bash -c 'archiver pause | grep -q "Copies to the secondary storages paused"' || die "$name: pause did not reach the worker"
  docker exec "$P-$name" bash -c 'archiver resume | grep -q "Copies to the secondary storages resumed"' || die "$name: resume did not reach the worker"
  docker exec "$P-$name" bash -c 'archiver stop | grep -q "Copies to the secondary storages stopped"' || die "$name: stop did not reach the worker"

  log "$name: restore from it"
  docker exec -e SNAPSHOT_ID="$id" -e STORAGE_TARGET=offsite -e LOCAL_DIR=/data/restore -e OVERWRITE=1 -e HASH_COMPARE=1 -e IGNORE_OWNERSHIP=1 \
    "$P-$name" archiver auto-restore || fail "$name" "restore from the copy exited non-zero"
  docker exec "$P-$name" grep -qx two /data/restore/file.txt || die "$name: the copy does not hold the latest revision"
  docker rm -f "$P-$name" >/dev/null
  echo "=== $1 OK as a copy ==="
}

if wanted minio || wanted minios || wanted wasabi; then
  log "Garage: single-node layout, a known key, the bucket"
  docker run -d --name "$P-garage" --network "$NET" --network-alias garage-api -v "$CFG":/cfg \
    "$GARAGE_IMAGE" /garage -c /cfg/garage.toml server >/dev/null || die "garage failed to start"
  for _ in $(seq 1 30); do garage status >/dev/null 2>&1 && break; sleep 1; done
  NODE="$(garage node id -q 2>/dev/null | cut -d@ -f1)"
  [ -n "$NODE" ] || die "garage never reported a node id"
  if ! { garage layout assign -z dc1 -c 1G "$NODE" && garage layout apply --version 1 \
    && garage key import --yes -n "$BUCKET" "$S3_KEY" "$S3_SECRET" && garage bucket create "$BUCKET" \
    && garage bucket allow --read --write --owner "$BUCKET" --key "$BUCKET"; } >/dev/null; then
    die "garage setup failed"
  fi
fi
S3CHECK=("TYPE=s3" "PROVIDER=Other" "ACCESS_KEY_ID=$S3_KEY" "SECRET_ACCESS_KEY=$S3_SECRET" "REGION=$REGION" "FORCE_PATH_STYLE=true")
s3_ready() { # NAME N: the S3 endpoint answers a signed listing
  [ "$2" = 1 ] || return 0
  docker exec "$P-$1" bash -c "for _ in \$(seq 1 60); do curl -fsS -o /dev/null --aws-sigv4 'aws:amz:$REGION:s3' --user '$S3_KEY:$S3_SECRET' '$S3_PROBE/$BUCKET/?list-type=2' && exit 0; sleep 1; done; exit 1"
}
s3case() { # TYPE ENDPOINT CHECK-ENDPOINT [EXTRA-CHECK-SETTING]
  FIELDS=("TYPE=$1" "S3_ENDPOINT=$2" "S3_BUCKETNAME=$BUCKET" "S3_REGION=$REGION" "S3_PATH=$1/%D")
  SECRETS=("S3_ID=$S3_KEY" "S3_SECRET=$S3_SECRET") PREP=s3_ready S3_PROBE="$3"
  run_case "$1" "$BUCKET/$1/%D" "${S3CHECK[@]}" "ENDPOINT=$3" "${@:4}"
}
wanted minio && s3case minio garage-api:3900 http://garage-api:3900
wanted minios && s3case minios garage https://garage
if wanted s3c; then
  docker run -d --name "$P-seaweed" --network "$NET" --network-alias seaweed-s3 -v "$CFG":/cfg:ro "$SEAWEED_IMAGE" \
    server -dir=/tmp -s3 -s3.port=8333 -s3.config=/cfg/seaweed.json >/dev/null || die "seaweedfs failed to start"
  for _ in $(seq 1 60); do
    docker run --rm --network "$NET" -e RCLONE_CONFIG_SW_TYPE=s3 -e RCLONE_CONFIG_SW_PROVIDER=SeaweedFS -e RCLONE_CONFIG_SW_ENDPOINT=http://seaweed-s3:8333 \
      -e RCLONE_CONFIG_SW_ACCESS_KEY_ID="$S3_KEY" -e RCLONE_CONFIG_SW_SECRET_ACCESS_KEY="$S3_SECRET" --entrypoint rclone "$IMAGE" \
      --config /dev/null mkdir "SW:$BUCKET" >/dev/null 2>&1 && break
    sleep 1
  done
  # The check signs V2 as well; the readiness probe signs V4, which SeaweedFS also takes.
  s3case s3c seaweed https://seaweed V2_AUTH=true
fi
if wanted wasabi; then
  FIELDS=(TYPE=wasabi WASABI_ENDPOINT=garage "WASABI_REGION=$REGION" "WASABI_BUCKETNAME=$BUCKET" "WASABI_PATH=wasabi/%D")
  SECRETS=("WASABI_KEY=$S3_KEY" "WASABI_SECRET=$S3_SECRET") PREP=s3_ready S3_PROBE=https://garage
  NO_MOVE="Wasabi's own MOVE request, which Garage does not serve" run_case wasabi "$BUCKET/wasabi/%D" "${S3CHECK[@]}" "ENDPOINT=https://garage"
fi

if wanted sftpc; then
  docker run -d --name "$P-sftp" --network "$NET" --network-alias sftp-server -v "$CFG":/cfg:ro \
    "$SFTP_IMAGE" sh -c 'mkdir -p /home/backup/.ssh/keys && cp /cfg/sshkeys/id_ed25519.pub /home/backup/.ssh/keys/ && exec /entrypoint backup::1001::upload' \
    >/dev/null || die "sftp server failed to start"
  # Duplicacy's SFTP storage needs its directory to exist already.
  for _ in $(seq 1 30); do
    docker exec "$P-sftp" sh -c 'id backup && mkdir -p /home/backup/upload/store /home/backup/upload/copy && chown -R backup /home/backup/upload' >/dev/null 2>&1 && break
    sleep 1
  done
  sftp_keys() {
    docker exec "$P-$1" bash -c '
      set -e
      install -m 600 /cfg/id_ed25519 /opt/archiver/keys/id_ed25519 && cp /cfg/sshkeys/id_ed25519.pub /opt/archiver/keys/
      mkdir -p /root/.ssh
      for _ in $(seq 1 30); do ssh-keyscan -T 3 sftp-server > /root/.ssh/known_hosts 2>/dev/null && [ -s /root/.ssh/known_hosts ] && exit 0; sleep 1; done
      exit 1'
  }
  FIELDS=(TYPE=sftpc SFTP_URL=sftp-server SFTP_USER=backup "SFTP_PATH=upload/%D") SECRETS=() PREP=sftp_keys
  run_case sftpc /upload/%D TYPE=sftp HOST=sftp-server USER=backup KEY_FILE=/opt/archiver/keys/id_ed25519 \
    KNOWN_HOSTS_FILE=/root/.ssh/known_hosts SHELL_TYPE=none
fi

if wanted webdav || wanted webdav-http; then
  # rclone, from the image under test, serves WebDAV; Caddy fronts it for the HTTPS type.
  # Duplicacy's WebDAV storage needs its directory to exist already.
  docker run -d --name "$P-davsrv" --network "$NET" --network-alias webdav-srv --entrypoint sh "$IMAGE" \
    -c 'mkdir -p /srv/dav/webdav/store /srv/dav/webdav/copy /srv/dav/webdav-http/store /srv/dav/webdav-http/copy && exec rclone serve webdav /srv/dav --addr :8080 --user davuser --pass davpass' >/dev/null \
    || die "webdav server failed to start"
fi
dav_ready() { # NAME N
  [ "$2" = 1 ] || return 0
  docker exec "$P-$1" bash -c "for _ in \$(seq 1 30); do curl -fsS -o /dev/null -u davuser:davpass -X PROPFIND -H 'Depth: 0' '$DAV_URL/' && exit 0; sleep 1; done; exit 1"
}
davcase() { # TYPE HOST CHECK-URL
  FIELDS=("TYPE=$1" "WEBDAV_HOST=$2" WEBDAV_USER=davuser "WEBDAV_PATH=$1/%D") SECRETS=(WEBDAV_PASSWORD=davpass) PREP=dav_ready DAV_URL="$3"
  local pass; pass="$(printf davpass | docker run --rm -i --entrypoint rclone "$IMAGE" obscure -)"
  run_case "$1" "/$1/%D" TYPE=webdav "URL=$3" USER=davuser "PASS=$pass"
}
wanted webdav-http && davcase webdav-http webdav-srv:8080 http://webdav-srv:8080
wanted webdav && davcase webdav webdav-tls https://webdav-tls
if wanted webdav-http; then
  log "a storage Duplicacy cannot open is explained: a missing directory, an unreachable host"
  start_archiver diagnose -e STORAGE_TARGET_1_NAME=offsite -e STORAGE_TARGET_1_TYPE=webdav-http -e STORAGE_TARGET_1_WEBDAV_HOST=webdav-srv:8080 \
    -e STORAGE_TARGET_1_WEBDAV_USER=davuser -e STORAGE_TARGET_1_WEBDAV_PATH=missing/dir
  secret diagnose storage_target_1_webdav_password davpass
  docker exec "$P-diagnose" archiver backup >/dev/null 2>&1 && die "a backup to a missing directory succeeded"
  docker exec "$P-diagnose" grep -q "\[ERROR\].*Storage 'offsite' (webdav-http): /missing/dir does not exist there. Create it first" /opt/archiver/logs/archiver.log \
    || fail diagnose "a missing directory is not explained"
  docker rm -f "$P-diagnose" >/dev/null
  start_archiver diagnose -e STORAGE_TARGET_1_NAME=offsite -e STORAGE_TARGET_1_TYPE=webdav-http -e STORAGE_TARGET_1_WEBDAV_HOST=no-such-host:8080 \
    -e STORAGE_TARGET_1_WEBDAV_USER=davuser -e STORAGE_TARGET_1_WEBDAV_PATH=x
  secret diagnose storage_target_1_webdav_password davpass
  docker exec "$P-diagnose" archiver backup >/dev/null 2>&1 && die "a backup to an unreachable host succeeded"
  docker exec "$P-diagnose" grep -q "\[ERROR\].*Storage 'offsite' (webdav-http) cannot be reached: .*no-such-host" /opt/archiver/logs/archiver.log \
    || fail diagnose "an unreachable host is not explained"
  docker rm -f "$P-diagnose" >/dev/null
  echo "=== failures to open a storage are explained ==="
fi

if wanted smb; then
  docker run -d --name "$P-samba" --network "$NET" --network-alias smb-server \
    -e ACCOUNT_backup=smbpass -e UID_backup=1001 \
    -e 'SAMBA_VOLUME_CONFIG_backup=[backup]; path=/shares/backup; valid users = backup; guest ok = no; read only = no; browseable = yes' \
    "$SAMBA_IMAGE" >/dev/null || die "samba failed to start"
  for _ in $(seq 1 30); do docker exec "$P-samba" sh -c 'mkdir -p /shares/backup/store /shares/backup/copy && chown -R backup /shares/backup' 2>/dev/null && break; sleep 1; done
  smb_ready() { docker exec "$P-$1" bash -c 'for _ in $(seq 1 30); do (echo > /dev/tcp/smb-server/445) 2>/dev/null && exit 0; sleep 1; done; exit 1'; }
  FIELDS=(TYPE=smb SMB_HOST=smb-server SMB_USER=backup SMB_SHARE=backup "SMB_PATH=%D") SECRETS=(SMB_PASSWORD=smbpass) PREP=smb_ready
  pass="$(printf smbpass | docker run --rm -i --entrypoint rclone "$IMAGE" obscure -)"
  run_case smb backup/%D TYPE=smb HOST=smb-server USER=backup "PASS=$pass"
fi

if wanted swift; then
  docker run -d --name "$P-saio" --network "$NET" --network-alias saio "$SWIFT_IMAGE" >/dev/null || die "swift failed to start"
  swift_ready() { # NAME N: the container exists
    [ "$2" = 1 ] || return 0
    docker exec "$P-$1" bash -c '
      for _ in $(seq 1 90); do
        RCLONE_CONFIG_SW_TYPE=swift RCLONE_CONFIG_SW_AUTH=http://saio:8080/auth/v1.0 RCLONE_CONFIG_SW_USER=test:tester RCLONE_CONFIG_SW_KEY=testing \
          rclone --config /dev/null mkdir SW:archiver 2>/dev/null && exit 0
        sleep 2
      done; exit 1'
  }
  FIELDS=(TYPE=swift 'SWIFT_URL=test:tester@saio:8080/auth/v1.0/archiver/%D?protocol=http') SECRETS=(SWIFT_KEY=testing) PREP=swift_ready
  run_case swift archiver/%D TYPE=swift AUTH=http://saio:8080/auth/v1.0 USER=test:tester KEY=testing
fi

if wanted azure; then
  docker run -d --name "$P-azurite" --network "$NET" --network-alias azurite -e AZURITE_ACCOUNTS="$AZ_ACCOUNT:$AZ_KEY" "$AZURITE_IMAGE" \
    azurite-blob --blobHost 0.0.0.0 --skipApiVersionCheck --loose >/dev/null || die "azurite failed to start"
  azure_ready() { # NAME N: both phases' containers exist
    [ "$2" = 1 ] || return 0
    docker exec -e RCLONE_CONFIG_AZ_TYPE=azureblob -e RCLONE_CONFIG_AZ_ACCOUNT="$AZ_ACCOUNT" -e RCLONE_CONFIG_AZ_KEY="$AZ_KEY" "$P-$1" bash -c '
      for _ in $(seq 1 30); do rclone --config /dev/null mkdir AZ:store 2>/dev/null && rclone --config /dev/null mkdir AZ:copy && exit 0; sleep 1; done; exit 1'
  }
  FIELDS=(TYPE=azure "AZURE_ACCOUNT=$AZ_ACCOUNT" "AZURE_CONTAINER=%D") SECRETS=("AZURE_KEY=$AZ_KEY") PREP=azure_ready
  run_case azure %D TYPE=azureblob "ACCOUNT=$AZ_ACCOUNT" "KEY=$AZ_KEY"
fi

if wanted gcs; then
  docker run -d --name "$P-fakegcs" --network "$NET" --network-alias fake-gcs "$FAKEGCS_IMAGE" \
    -scheme http -port 4443 -backend memory -public-host storage.googleapis.com -external-url https://storage.googleapis.com >/dev/null || die "fake-gcs failed to start"
  # A service account of the test's own: a real key signs the assertion the stand-in grants.
  gcs_account() { # NAME N
    docker exec "$P-$1" bash -c '
      set -e
      openssl genrsa -out /tmp/sa.pem 2048 2>/dev/null
      key=$(awk "{printf \"%s\\\\n\", \$0}" /tmp/sa.pem)
      printf "{\"type\":\"service_account\",\"project_id\":\"archiver-test\",\"private_key_id\":\"1\",\"private_key\":\"%s\",\"client_email\":\"archiver@archiver-test.iam.gserviceaccount.com\",\"client_id\":\"1\",\"token_uri\":\"https://oauth2.googleapis.com/token\"}" "$key" > /run/secrets/storage_target_$1_gcs_token
      [ "$1" = 1 ] || exit 0
      for _ in $(seq 1 30); do curl -fsS -o /dev/null -X POST -H "Content-Type: application/json" -d "{\"name\":\"$2\"}" "https://www.googleapis.com/storage/v1/b?project=archiver-test" && exit 0; sleep 1; done
      exit 1
    ' _ "$2" "$BUCKET"
  }
  FIELDS=(TYPE=gcs "GCS_BUCKETNAME=$BUCKET" "GCS_PATH=%D") SECRETS=() PREP=gcs_account
  run_case gcs "$BUCKET/%D" TYPE="google cloud storage" SERVICE_ACCOUNT_FILE=/run/secrets/storage_target_1_gcs_token BUCKET_POLICY_ONLY=true
fi

# Dropbox has no emulator (its round trip runs against a real account, ADRs 25, 27). This proves the wiring: the
# app's credentials reach Duplicacy, which refreshes at Dropbox's own token endpoint with
# them and calls the API with the token it got, never duplicacy.com.
if wanted dropbox; then
  start_archiver dropbox -e STORAGE_TARGET_1_NAME=offsite -e STORAGE_TARGET_1_TYPE=dropbox -e STORAGE_TARGET_1_DROPBOX_PATH=backups \
    -e STORAGE_TARGET_1_DROPBOX_APP_KEY=app-key
  secret dropbox storage_target_1_dropbox_app_secret app-secret
  secret dropbox storage_target_1_dropbox_token refresh-token
  log "dropbox: backup against the stand-in (expected to fail: every API call is refused)"
  docker exec "$P-dropbox" timeout 120 archiver backup >/dev/null 2>&1 && die "dropbox: a backup against the refusing stand-in succeeded"
  logs="$(docker logs "$P-proxy" 2>&1)"
  basic="Basic $(printf 'app-key:app-secret' | base64)"
  grep -qF "$basic" <<<"$(grep '"uri":"/oauth2/token"' <<<"$logs")" || die "dropbox: no token refresh with the app's credentials"
  if ! grep -qF 'Bearer fake-access' <<<"$(grep '"uri":"/2/' <<<"$logs")"; then
    grep -o '"uri":"[^"]*"' <<<"$logs" >&2
    fail dropbox "no API call with the refreshed token"
  fi
  docker rm -f "$P-dropbox" >/dev/null
  echo "=== dropbox OK (token wiring) ==="
fi

echo "=== BACKENDS OK: ${WANT[*]} ==="
