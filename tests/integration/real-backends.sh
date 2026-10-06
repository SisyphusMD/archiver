#!/usr/bin/env bash
# Round trips through real services (ADRs 25, 27: weekly, and before every release), for the
# types no emulator covers and for
# what an emulator cannot do (Wasabi's own MOVE request). For each backend whose credentials are
# present, the same two phases as tests/integration/backends.sh:
#   1. the service as the primary, with a local copy: backup, inline copy, the recovery kit
#      placed on it, a restore from it, maintenance (check, prune to 7 days);
#   2. the service as the copy of a local primary: an inline copy, then under the daemon its
#      copy worker's hand-off, check, mirroring of a revision local pruned and exhaustive prune,
#      pause, resume and stop, a restore from it, and maintenance (prune -all to 7 days).
# Phase 1 backs up one snapshot ID every run, so its 7-day prune keeps the primary storage
# bounded. Phase 2 needs a new ID each night (a copy cannot continue an ID whose history its
# fresh local primary lacks), and Duplicacy keeps every ID's newest revision, so the copy
# storage gains one revision of a few kilobytes a run.
# Each backend keeps its storages in a dedicated test account: nothing outside them is written
# or deleted.
#
# Credentials come from NIGHTLY_<TYPE> environment variables (Forgejo secrets, upper-case type
# with - as _), one line per field of the storage target: "FIELD=value" for a setting and
# "secret FIELD=value" for a secret. %D in a value stands for the storage's directory, which
# differs between the phases ("primary", "copy"): put it in the type's path, container or
# bucket field. A backend without credentials is skipped, not failed. NIGHTLY_RSA_KEY is the
# RSA private key (PEM, passphrase "nightlypassphrase") every night uses: a storage keeps the
# public key it was created with, so a new key each run could not restore.
#
# HOST-DRIVEN: run on the docker host; failures never gate a PR, but stop a release (ADR 27).
#
#   IMAGE=archiver:dev NIGHTLY_RSA_KEY=... NIGHTLY_B2='...' bash tests/integration/real-backends.sh [type ...]

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
P="archiver-nightly-$$"
RECOVERY="nightly-recovery-pw"
LG=/opt/archiver/logs
ALL=(b2 b2-custom gcd one odb dropbox storj fabric azure gcs wasabi)
WANT=("$@")
[ ${#WANT[@]} -gt 0 ] || WANT=("${ALL[@]}")

log() { printf '>>> %s\n' "$*"; }
cleanup() {
  docker ps -aq --filter "name=^$P-" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker volume ls -q --filter "name=^$P-" | xargs -r docker volume rm >/dev/null 2>&1 || true
}
trap cleanup EXIT
# step CONTAINER DESCRIPTION COMMAND: run COMMAND in the container; on failure print the logs.
step() {
  local c="$1" what="$2"; shift 2
  docker exec "$c" bash -c "$*" && return 0
  echo "FAIL: $what" >&2
  docker exec "$c" bash -c "tail -n 25 $LG/archiver.log $LG/maintenance.log $LG/copies.log 2>/dev/null" >&2
  return 1
}
until_in() { # CONTAINER SECONDS COMMAND
  local c="$1" secs="$2"; shift 2
  for _ in $(seq 1 "$secs"); do docker exec "$c" bash -c "$*" >/dev/null 2>&1 && return 0; sleep 1; done
  return 1
}

# start TYPE CONTAINER N DIR RUN BUNDLE [docker run args...]: a container with the backend as
# target N (directory DIR), the nightly keys and secrets, and fixtures in /data/RUN.
start() {
  local typ="$1" c="$2" n="$3" d="$4" run="$5" bundle="$6" line field value
  shift 6
  local envs=(-e "STORAGE_TARGET_${n}_NAME=nightly" -e "STORAGE_TARGET_${n}_TYPE=$typ" -e "SERVICE_DIRECTORIES=/data/$run/"
    -e PRUNE_BACKUPS=true -e "PRUNE_KEEP=-keep 0:7" "$@")
  local secrets=()
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    line="${line//%D/$d}"
    if [[ $line == "secret "* ]]; then secrets+=("${line#secret }"); else envs+=(-e "STORAGE_TARGET_${n}_$line"); fi
  done <<<"$bundle"
  docker rm -f "$c" >/dev/null 2>&1
  # NIGHTLY_DOCKER_ARGS: extra docker run arguments, for trying this script with a stand-in
  # service (a volume mounted for a local-type "service" that persists between nights).
  # shellcheck disable=SC2086
  docker run -d --name "$c" --hostname "nightly-$typ" --cap-drop ALL --cap-add DAC_OVERRIDE "${envs[@]}" ${NIGHTLY_DOCKER_ARGS:-} \
    --entrypoint bash "$IMAGE" -c 'sleep 7200' >/dev/null || return 1
  printf '%s\n' "$NIGHTLY_RSA_KEY" | docker exec -i "$c" bash -c "
    set -e
    mkdir -p /opt/archiver/keys /run/secrets /data/$run /local
    cat > /opt/archiver/keys/private.pem && chmod 600 /opt/archiver/keys/private.pem
    openssl rsa -in /opt/archiver/keys/private.pem -passin pass:nightlypassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
    printf nightlypassword > /run/secrets/storage_password
    printf nightlypassphrase > /run/secrets/rsa_passphrase
    printf '$RECOVERY' > /run/secrets/recovery_password
    echo 'nightly $typ' > /data/$run/file.txt
    head -c 2048 /dev/urandom > /data/$run/blob.bin
  " || return 1
  for line in "${secrets[@]}"; do
    field="${line%%=*}" value="${line#*=}"
    printf '%s' "$value" | docker exec -i "$c" sh -c "cat > /run/secrets/storage_target_${n}_$(printf '%s' "$field" | tr '[:upper:]' '[:lower:]')" || return 1
  done
}

restore_from() { # CONTAINER STORAGE ID EXPECT
  step "$1" "restore from $2" "SNAPSHOT_ID=$3 STORAGE_TARGET=$2 LOCAL_DIR=/data/restore OVERWRITE=1 HASH_COMPARE=1 IGNORE_OWNERSHIP=1 archiver auto-restore >/dev/null" &&
    step "$1" "the restore holds the latest revision" "grep -qx '$4' /data/restore/file.txt"
}

# primary TYPE BUNDLE: phase 1.
primary() {
  local typ="$1" c="$P-$1" run=nightly
  start "$typ" "$c" 1 primary "$run" "$2" -v "$P-$typ-local:/local" -e STORAGE_TARGET_2_NAME=localcopy -e STORAGE_TARGET_2_TYPE=local -e STORAGE_TARGET_2_LOCAL_PATH=/local || return 1
  log "$typ as the primary: backup and copy"
  step "$c" "backup" "archiver backup >/dev/null" || return 1
  log "$typ: the kit placed on it, and the local copy's decrypted with only the password"
  step "$c" "kit placed on the service" "grep -q \"Recovery kit updated on storage 'nightly'\" $LG/archiver.log" || return 1
  step "$c" "kit decryptable" "openssl enc -d -aes-256-cbc -pbkdf2 -pass 'pass:$RECOVERY' -in /local/archiver-recovery-kit-nightly-$typ.tar.enc | tar -xf - -C /tmp && grep -q '^STORAGE_TARGET_1_TYPE=$typ\$' /tmp/archiver.env" || return 1
  log "$typ: restore from it, then maintenance"
  restore_from "$c" nightly "nightly-$typ-$run" "nightly $typ" || return 1
  step "$c" "maintenance" "archiver maintenance >/dev/null" || return 1
  docker rm -f "$c" >/dev/null
}

# copy TYPE BUNDLE: phase 2.
copy() {
  local typ="$1" c="$P-$1-copy" run id
  run="run-$(date -u +%Y%m%d%H%M%S)"
  id="nightly-$1-$run"
  # The local primary is the copy phase 1 made: bit-identical to the service's persistent primary
  # storage, so every night's local storage matches the copy storage already on the service.
  start "$typ" "$c" 2 copy "$run" "$2" -v "$P-$typ-local:/local" -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/local || return 1
  log "$typ as a copy: inline copy"
  step "$c" "backup with an inline copy" "archiver backup >/dev/null && grep -q 'Copy to nightly storage completed' $LG/archiver.log" || return 1
  log "$typ: its copy worker takes a backup's copy and checks it"
  docker exec -d "$c" sh -c 'BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/daemon.out 2>&1'
  until_in "$c" 120 'archiver status | grep -q "nightly: caught up"' || { step "$c" "worker caught up" false; return 1; }
  step "$c" "backup handed off" "sleep 1.1; echo two > /data/$run/file.txt; archiver backup >/dev/null 2>&1; grep -q 'run in the background' $LG/archiver.log" || return 1
  until_in "$c" 300 "grep -q 'Copy to nightly storage completed' $LG/copies.log" || { step "$c" "worker copied" false; return 1; }
  until_in "$c" 600 "grep -q 'Check of nightly storage completed' $LG/copies.log" || { step "$c" "worker checked" false; return 1; }
  log "$typ: a revision local prunes is mirrored off it, then an exhaustive prune"
  step "$c" "local prune of revision 1" "cd /data/$run && DUPLICACY_LOCAL_PASSWORD=nightlypassword DUPLICACY_LOCAL_RSA_PASSPHRASE=nightlypassphrase duplicacy prune -storage local -id $id -r 1 >/dev/null" || return 1
  step "$c" "mirroring planned" "archiver mirror --dry-run | grep -q 'nightly: delete $id revisions 1\$'" || return 1
  docker exec "$c" archiver daemon ctl local-changed >/dev/null
  until_in "$c" 300 "! archiver mirror --dry-run | grep -q 'nightly: delete'" || { step "$c" "revision 1 mirrored" false; return 1; }
  # The worker may have pruned exhaustively while catching up; wait for the one asked for here.
  local before
  before=$(docker exec "$c" grep -c 'Exhaustive prune of nightly storage completed' "$LG/copies.log")
  step "$c" "exhaustive maintenance" "archiver maintenance exhaustive >/dev/null" || return 1
  until_in "$c" 600 "[ \$(grep -c 'Exhaustive prune of nightly storage completed' $LG/copies.log) -gt $before ]" || { step "$c" "worker pruned exhaustively" false; return 1; }
  log "$typ: pause, resume and stop reach its worker"
  step "$c" "pause, resume, stop" "archiver pause | grep -q paused && archiver resume | grep -q resumed && archiver stop | grep -q stopped" || return 1
  docker exec "$c" pkill -TERM -f 'archiver daemon'
  sleep 3
  log "$typ: restore from it, then maintenance keeps it to 7 days"
  restore_from "$c" nightly "$id" two || return 1
  step "$c" "maintenance" "archiver maintenance >/dev/null" || return 1
  docker rm -f "$c" >/dev/null
}

[ -n "${NIGHTLY_RSA_KEY:-}" ] || { echo "NIGHTLY_RSA_KEY is not set: nothing to run" >&2; exit 1; }
failed=() passed=() skipped=()
for typ in "${WANT[@]}"; do
  var="NIGHTLY_$(printf '%s' "$typ" | tr '[:lower:]-' '[:upper:]_')"
  if [ -z "${!var:-}" ]; then skipped+=("$typ"); continue; fi
  if primary "$typ" "${!var}" && copy "$typ" "${!var}"; then
    passed+=("$typ"); echo "=== $typ OK ==="
  else
    failed+=("$typ"); echo "FAIL: $typ" >&2
  fi
done
echo "passed: ${passed[*]:-none}; failed: ${failed[*]:-none}; skipped (no credentials): ${skipped[*]:-none}"
[ ${#failed[@]} -eq 0 ]
