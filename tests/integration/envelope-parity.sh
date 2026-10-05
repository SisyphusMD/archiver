#!/usr/bin/env bash
# The Go envelope is the bash envelope: for the same configuration both give the same
# fingerprint (so a page confirmed under either stays current under the other) and write
# byte-identical HTML and PDF. Deleted with the bash envelope.
#
#   docker run -i --rm --hostname ep-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/envelope-parity.sh

set -uo pipefail

S=/run/secrets
log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "four storages: local, SFTP with a break-glass key and user, B2 break-glass, S3 backup credentials"
mkdir -p /opt/archiver/keys "$S" /store
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die genrsa
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die pubout
ssh-keygen -q -t ed25519 -N '' -f /opt/archiver/keys/id_ed25519 -C archiver
ssh-keygen -q -t ed25519 -N '' -f /tmp/bg -C breakglass
printf 'testpassword' >"$S/storage_password"; printf 'rp' >"$S/rsa_passphrase"
printf 'recovery (pw) & <stuff> \\ "quoted"' >"$S/recovery_password"
cp /tmp/bg "$S/storage_target_2_breakglass_ssh_key"
printf 'k1' >"$S/storage_target_3_b2_id"; printf 'key1' >"$S/storage_target_3_b2_key"
printf 'bgid' >"$S/storage_target_3_breakglass_b2_id"; printf 'bgkey' >"$S/storage_target_3_breakglass_b2_key"
printf 'AKID' >"$S/storage_target_4_s3_id"; printf 'sécret' >"$S/storage_target_4_s3_secret"
export SERVICE_DIRECTORIES=/data/
export STORAGE_TARGET_1_NAME=lokal STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH=/store/
export STORAGE_TARGET_2_NAME=nas STORAGE_TARGET_2_TYPE=sftp STORAGE_TARGET_2_SFTP_URL=nas.lan STORAGE_TARGET_2_SFTP_PORT=2222 STORAGE_TARGET_2_SFTP_USER=backup \
  STORAGE_TARGET_2_SFTP_PATH=/volume1/archiver STORAGE_TARGET_2_BREAKGLASS_SFTP_USER=reader
export STORAGE_TARGET_3_NAME=b2 STORAGE_TARGET_3_TYPE=b2 STORAGE_TARGET_3_B2_BUCKETNAME=bucket
export STORAGE_TARGET_4_NAME=wasabi STORAGE_TARGET_4_TYPE=s3 STORAGE_TARGET_4_S3_BUCKETNAME=bkt STORAGE_TARGET_4_S3_ENDPOINT=s3.example.com

compare() {
  bash /opt/archiver/lib/scripts/envelope.sh /tmp/bash-env >/tmp/bash.out 2>&1 || { cat /tmp/bash.out; die "the bash envelope failed"; }
  bfp="$(cut -d' ' -f1 /opt/archiver/logs/.envelope-written)"
  archiver envelope /tmp/go-env >/tmp/go.out 2>&1 || { cat /tmp/go.out; die "the Go envelope failed"; }
  gfp="$(cut -d' ' -f1 /opt/archiver/logs/.envelope-written)"
  [ "$bfp" = "$gfp" ] || die "$1: fingerprints differ: bash $bfp, Go $gfp"
  cmp /tmp/bash-env/envelope-ep-host.html /tmp/go-env/envelope-ep-host.html || die "$1: the HTML differs"
  if [ -e /tmp/bash-env/envelope-ep-host.pdf ]; then
    cmp /tmp/bash-env/envelope-ep-host.pdf /tmp/go-env/envelope-ep-host.pdf || die "$1: the PDF differs"
  else
    [ ! -e /tmp/go-env/envelope-ep-host.pdf ] || die "$1: Go wrote a PDF bash refused"
  fi
  [ "$(stat -c %a /tmp/go-env/envelope-ep-host.html)" = 600 ] || die "$1: the HTML is not private"
}

log "the same page from both"
compare "four storages"
[ -s /tmp/go-env/envelope-ep-host.pdf ] && head -c 8 /tmp/go-env/envelope-ep-host.pdf | grep -q '%PDF-1.4' || die "no PDF to compare"
grep -q 'Too long for a QR code' /tmp/go-env/envelope-ep-host.html && die "a QR code that fits was refused"

log "a page confirmed under bash is current under Go, and confirms under Go"
bash /opt/archiver/lib/scripts/envelope.sh confirm >/dev/null || die "bash confirm"
archiver recovery-kit >/dev/null 2>&1 || true   # the kit step runs the Go envelope check
grep -q '^Envelope: current' <<<"$(archiver status)" || die "a bash-confirmed page is not current under Go: $(archiver status | grep Envelope)"
archiver envelope confirm >/dev/null || die "Go confirm of a bash-written page"

log "text the PDF fonts cannot show: neither writes a PDF"
printf 'key \xe2\x9c\x93 check' >"$S/storage_target_4_s3_secret"
rm -rf /tmp/bash-env /tmp/go-env
compare "unprintable"
[ ! -e /tmp/go-env/envelope-ep-host.pdf ] || die "a PDF with unprintable text"

log "a credential too long for a QR code"
head -c 3000 /dev/urandom | base64 -w0 >"$S/storage_target_4_s3_secret"
rm -rf /tmp/bash-env /tmp/go-env
compare "long credential"
grep -q 'Too long for a QR code' /tmp/go-env/envelope-ep-host.html || die "an oversized QR code was not refused"

echo "=== ENVELOPE-PARITY OK: same fingerprint, HTML and PDF as the bash envelope; confirmations carry across ==="
