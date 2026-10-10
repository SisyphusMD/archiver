#!/usr/bin/env bash
# Podman parity (ADR 5): the image runs under rootful Podman with the hardened capabilities
# and backs up; a hook drives the host's containers through Podman's socket mounted as
# docker.sock (the docker CLI against Podman's API); the host-side `archiver` command works
# through podman; and `podman stop` stops gracefully.
#
# HOST-DRIVEN, and needs a privileged container for Podman itself (CI's DinD allows one).
#
#   IMAGE=archiver:dev bash tests/integration/podman.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
# renovate: datasource=docker depName=quay.io/podman/stable
PODMAN_IMAGE=quay.io/podman/stable:v5.6.2@sha256:b4bdf91d79ef0396ec1c070faa395b8e879fd4da5b161a7522882619026d5fa4
HOST="archiver-podman-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; docker exec "$HOST" podman logs archiver 2>&1 | tail -20 >&2; exit 1; }
# -v: the Podman image declares volumes for its storage, which holds the loaded image.
cleanup() { docker rm -fv "$HOST" >/dev/null 2>&1 || true; }
trap cleanup EXIT
on_host() { docker exec -i "$HOST" "$@"; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }

log "a Podman host, with the image loaded and Podman's API socket serving"
docker run -d --privileged --name "$HOST" "$PODMAN_IMAGE" sleep infinity >/dev/null || die "the Podman host did not start"
docker save "$IMAGE" | on_host podman load -q >/dev/null || die "podman load failed"
on_host podman tag "$IMAGE" archiver:test || die "podman tag failed"
on_host sh -c 'mkdir -p /run/podman && (podman system service --time=0 unix:///run/podman/podman.sock >/dev/null 2>&1 &)'
wait_for 'on_host test -S /run/podman/podman.sock' 30 || die "the Podman socket never appeared"

log "keys, secrets, a service whose hook uses the socket, and a container for it to reach"
docker run --rm --entrypoint sh "$IMAGE" -c '
  set -e; mkdir -p /out/secrets
  openssl genrsa -aes256 -passout pass:pp -out /out/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /out/secrets/rsa_private_key -passin pass:pp -pubout -out /out/secrets/rsa_public_key 2>/dev/null
  printf testpassword > /out/secrets/storage_password; printf pp > /out/secrets/rsa_passphrase
  tar -C /out -c secrets' | on_host tar -C /srv -x || die "secrets"
on_host sh -c '
  set -e; chmod 600 /srv/secrets/*; mkdir -p /srv/data/app /srv/store
  echo data > /srv/data/app/file
  printf "#!/bin/sh\nset -e\ndocker exec db touch /tmp/dumped\ndocker ps --format \"{{.Names}}\" > podman-ps.txt\n" > /srv/data/app/pre-backup
  chmod 755 /srv /srv/data /srv/data/app /srv/data/app/pre-backup
  podman run -d --name db --entrypoint sleep archiver:test infinity >/dev/null' || die "the service setup failed"

log "archiver under Podman with the hardened capabilities"
on_host podman run -d --name archiver --uts private --hostname pd-host \
  --cap-drop ALL --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER --security-opt no-new-privileges \
  -v /srv/secrets:/run/secrets:ro -v /srv/data:/data -v /srv/store:/store \
  -v /run/podman/podman.sock:/var/run/docker.sock \
  -e SERVICE_DIRECTORIES=/data/app -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/store \
  archiver:test >/dev/null || die "podman run failed"
wait_for 'on_host podman logs archiver 2>&1 | grep -q "Container is ready"' 60 || die "the container never became ready"

log "the host-side archiver command, through podman"
on_host podman cp archiver:/opt/archiver/host/archiver /usr/local/bin/archiver || die "podman cp of the host command failed"
on_host archiver backup >/tmp/podman-backup.out 2>&1 || { cat /tmp/podman-backup.out; die "archiver backup through the host command failed"; }
on_host test -e /srv/store/snapshots/pd-host-app/1 || die "the backup made no revision"
on_host grep -qx db /srv/data/app/podman-ps.txt || die "the hook did not reach Podman through the socket"
on_host podman exec db test -e /tmp/dumped || die "the hook's docker exec did not reach the db container"
on_host archiver status >/tmp/podman-status.out 2>&1 || { cat /tmp/podman-status.out; die "archiver status through the host command failed"; }
grep -q "Backup health" /tmp/podman-status.out || { cat /tmp/podman-status.out; die "archiver status printed no backup health"; }

log "podman stop stops it gracefully"
on_host podman stop -t 120 archiver >/dev/null || die "podman stop failed"
[ "$(on_host podman inspect -f '{{.State.ExitCode}}' archiver)" = 0 ] || die "the container did not exit 0 on podman stop"

echo "PASS: podman"
