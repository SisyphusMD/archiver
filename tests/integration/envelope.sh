#!/usr/bin/env bash
# The break-glass envelope and the kit's extra paths: the page names every storage with a
# credential that reads the kit (a break-glass credential when set, else the backup one marked
# FULL ACCESS), both files are owner-only and the PDF is well-formed, a confirmed envelope goes
# stale when something on it changes, and RECOVERY_KIT_EXTRA_PATHS land inside the kit.
#
#   docker run -i --rm --hostname ev-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/envelope.sh

set -uo pipefail

STORE=/backup-store
SECRETS_DIR=/run/secrets
OUT=/tmp/envelope
LOG=/opt/archiver/logs/archiver.log
KIT="archiver-recovery-kit-$(hostname).tar.enc"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "keys and secrets, with three storages: local, sftp (no break-glass key), b2 (break-glass key)"
mkdir -p /opt/archiver/keys /opt/archiver/logs "$SECRETS_DIR" "$STORE" /data/services/app
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
ssh-keygen -q -t ed25519 -N "" -f /opt/archiver/keys/id_ed25519 || die "ssh-keygen"
printf 'storagepassword1' >"$SECRETS_DIR/storage_password"
printf 'rp' >"$SECRETS_DIR/rsa_passphrase"
printf ' pass\\nword(x)  two ' >"$SECRETS_DIR/recovery_password"
printf 'RW-KEY-ID' >"$SECRETS_DIR/storage_target_3_b2_id"
printf 'RW-APP-KEY' >"$SECRETS_DIR/storage_target_3_b2_key"
printf 'RO-KEY-ID' >"$SECRETS_DIR/storage_target_3_breakglass_b2_id"
printf 'RO-APP-KEY<&>"' >"$SECRETS_DIR/storage_target_3_breakglass_b2_key"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export STORAGE_TARGET_2_NAME=friend STORAGE_TARGET_2_TYPE=sftp STORAGE_TARGET_2_SFTP_URL=nas.example.com
export STORAGE_TARGET_2_SFTP_PORT=32928 STORAGE_TARGET_2_SFTP_USER=cody STORAGE_TARGET_2_SFTP_PATH=cody/duplicacy
export STORAGE_TARGET_3_NAME=backblaze STORAGE_TARGET_3_TYPE=b2 STORAGE_TARGET_3_B2_BUCKETNAME=dup-bucket

log "archiver envelope writes an owner-only HTML page and PDF, even over readable files"
HTML="$OUT/envelope-$(hostname).html"; PDF="$OUT/envelope-$(hostname).pdf"
mkdir -p "$OUT" && touch "$HTML" "$PDF" && chmod 644 "$HTML" "$PDF"
archiver envelope "$OUT" >/tmp/env.out 2>&1 || { cat /tmp/env.out; die "archiver envelope failed"; }
for f in "$HTML" "$PDF"; do
  [ -s "$f" ] || die "missing $f"
  [ "$(stat -c %a "$f")" = 600 ] || die "$f is not owner-only"
done
grep -q "PLAINTEXT" /tmp/env.out || die "no plaintext warning"

log "the page carries what a recovery needs, and the right credential per storage"
grep -qF '<pre> pass\nword(x)  two </pre>' "$HTML" || die "the recovery password is not on the page exactly"
grep -q "openssl enc -d -aes-256-cbc -pbkdf2 -in ${KIT}" "$HTML" || die "no decrypt command"
grep -q "Kit: ${STORE}/${KIT}" "$HTML" || die "local kit location missing"
grep -q "Kit: /cody/duplicacy/${KIT}" "$HTML" || die "sftp kit location missing"
grep -q 'Host: nas.example.com Port: 32928 User: cody' "$HTML" || die "sftp connection details missing"
grep -q 'BEGIN OPENSSH PRIVATE KEY' "$HTML" || die "sftp has no break-glass key, so the backup key must be on the page"
[ "$(grep -c 'FULL ACCESS' "$HTML")" = 1 ] || die "exactly the sftp storage should be marked FULL ACCESS"
grep -q 'RO-APP-KEY&lt;&amp;&gt;&quot;' "$HTML" || die "the b2 break-glass key is missing or badly escaped: $(grep -o 'RO-APP-KEY[^<]*' "$HTML" | head -1)"
grep -q 'RW-APP-KEY\|RW-KEY-ID' "$HTML" && die "the b2 backup key is on the page despite a break-glass key"
[ "$(grep -c '<svg' "$HTML")" -ge 3 ] || die "expected a QR code for the password, the sftp key and the b2 key"

log "the PDF is well-formed: every cross-reference offset points at its object"
head -c 8 "$PDF" | grep -q '^%PDF-1.4' || die "no PDF header"
tail -c 6 "$PDF" | grep -q '%%EOF' || die "no %%EOF"
startxref=$(tail -n 2 "$PDF" | head -1)
[ "$(tail -c +$((startxref + 1)) "$PDF" | head -c 4)" = xref ] || die "startxref does not point at the xref table"
n=0
while read -r off _ kind; do
  [ "$kind" = n ] || continue
  n=$((n + 1))
  [ "$(tail -c +$((10#$off + 1)) "$PDF" | head -c ${#n})" = "$n" ] || die "xref entry $n points at the wrong offset"
done < <(tail -c +$((startxref + 1)) "$PDF" | sed -n '4,$p' | sed '/^trailer/,$d')
[ "$n" -ge 7 ] || die "only $n objects"
grep -aq 'RO-APP-KEY' "$PDF" || die "the PDF lacks the b2 break-glass key"
grep -aqF '( pass\\nword\(x\)  two ) Tj' "$PDF" || die "the PDF does not carry the recovery password exactly (spaces, backslash, parentheses)"

log "an sftp break-glass key and its own account replace the backup key and user"
ssh-keygen -q -t ed25519 -N "" -C breakglass -f /tmp/bg_key || die "ssh-keygen"
cp /tmp/bg_key "$SECRETS_DIR/storage_target_2_breakglass_ssh_key"
export STORAGE_TARGET_2_BREAKGLASS_SFTP_USER=readonly
archiver envelope "$OUT" >/dev/null 2>&1 || die "envelope with an sftp break-glass key failed"
grep -q 'Host: nas.example.com Port: 32928 User: readonly' "$HTML" || die "the break-glass sftp user is not on the page"
grep -q "$(sed -n 4p /tmp/bg_key)" "$HTML" || die "the break-glass sftp key is not on the page"
grep -q "$(sed -n 4p /opt/archiver/keys/id_ed25519)" "$HTML" && die "the backup sftp key is on the page despite a break-glass key"
grep -q 'FULL ACCESS' "$HTML" && die "a storage is marked FULL ACCESS although each has a break-glass credential"
rm -f "$SECRETS_DIR/storage_target_2_breakglass_ssh_key"; unset STORAGE_TARGET_2_BREAKGLASS_SFTP_USER
archiver envelope "$OUT" >/dev/null 2>&1 || die "regenerating the envelope failed"

log "confirming refuses a page written before the configuration changed"
printf 'RO-APP-KEY-2' >"$SECRETS_DIR/storage_target_3_breakglass_b2_key"
archiver envelope confirm >/tmp/confirm.out 2>&1 && die "confirmed a page whose credentials have since changed"
grep -q 'changed after the envelope was written' /tmp/confirm.out || die "no explanation: $(cat /tmp/confirm.out)"
printf 'RO-APP-KEY<&>"' >"$SECRETS_DIR/storage_target_3_breakglass_b2_key"

log "confirming records what was printed; status shows it current"
archiver envelope confirm >/tmp/confirm.out 2>&1 || { cat /tmp/confirm.out; die "confirm failed"; }
grep -q '^Envelope: current, confirmed printed' <<<"$(archiver status)" || die "status does not show a current envelope: $(archiver status | grep Envelope)"
HC="$(archiver healthcheck 2>&1)"
grep -q 'Envelope:' <<<"$HC" && die "healthcheck warned about a current envelope"

log "the kit carries RECOVERY_KIT_EXTRA_PATHS; a missing one is reported, the rest still go out"
export STORAGE_TARGET_COUNT=1
unset STORAGE_TARGET_2_NAME STORAGE_TARGET_2_TYPE STORAGE_TARGET_2_SFTP_URL STORAGE_TARGET_2_SFTP_PORT STORAGE_TARGET_2_SFTP_USER STORAGE_TARGET_2_SFTP_PATH
unset STORAGE_TARGET_3_NAME STORAGE_TARGET_3_TYPE STORAGE_TARGET_3_B2_BUCKETNAME
rm -f "$SECRETS_DIR"/storage_target_3_*
mkdir -p /site/runbook
echo "step 1: download the kit" >/site/runbook/DR-RUNBOOK.md
echo "contacts" >"/site/runbook/Who To Call.md"
echo "restore helper" >/site/restore-common.sh
mkdir -p /x /other && echo "x two" >/x/restore-common.sh.2 && echo "other helper" >/other/restore-common.sh
export RECOVERY_KIT_EXTRA_PATHS=$'/site/runbook\n/site/restore-common.sh:/x/restore-common.sh.2:/other/restore-common.sh:/site/missing\n'

: >"$STORE/config"   # a storage root, where the kit belongs
archiver recovery-kit >/tmp/kit.out 2>&1 || { cat /tmp/kit.out; die "recovery-kit failed"; }
grep -q 'RECOVERY_KIT_EXTRA_PATHS entry /site/missing does not exist' "$LOG" || die "the missing extra path was not reported"
mkdir -p /tmp/kit && openssl enc -d -aes-256-cbc -pbkdf2 -in "$STORE/$KIT" -pass file:"$SECRETS_DIR/recovery_password" | tar -xf - -C /tmp/kit || die "kit does not decrypt"
grep -q 'step 1' /tmp/kit/extra/runbook/DR-RUNBOOK.md || die "the extra directory is not in the kit"
grep -q 'restore helper' /tmp/kit/extra/restore-common.sh || die "the extra file is not in the kit"
grep -q 'extra/ in this kit' /tmp/kit/RECREATE.txt || die "RECREATE.txt does not point to extra/"
grep -q 'x two' /tmp/kit/extra/restore-common.sh.2 || die "an extra named like a renamed duplicate was overwritten"
grep -q 'other helper' /tmp/kit/extra/restore-common.sh.3 || die "a second extra with the same name was not kept"
grep -qx 'RECOVERY_KIT_EXTRA_PATHS=/site/runbook:/site/restore-common.sh:/x/restore-common.sh.2:/other/restore-common.sh:/site/missing:' /tmp/kit/archiver.env || die "newline-separated extra paths were not kept on one line: $(grep -A2 EXTRA /tmp/kit/archiver.env)"
grep -q 'contacts' "/tmp/kit/extra/runbook/Who To Call.md" || die "a file name with spaces is not in the kit"

log "a change to an extra with spaces in its name re-uploads the kit"
before="$(sha256sum "$STORE/$KIT" | cut -c1-64)"
echo "new contacts" >"/site/runbook/Who To Call.md"
archiver recovery-kit >/dev/null 2>&1 || die "kit run after the change failed"
[ "$(sha256sum "$STORE/$KIT" | cut -c1-64)" != "$before" ] || die "the kit was not re-uploaded after an extra changed"

log "a restore helper becoming executable re-uploads the kit"
before="$(sha256sum "$STORE/$KIT" | cut -c1-64)"
chmod 755 /site/restore-common.sh
archiver recovery-kit >/dev/null 2>&1 || die "kit run after the mode change failed"
[ "$(sha256sum "$STORE/$KIT" | cut -c1-64)" != "$before" ] || die "the kit was not re-uploaded after an extra's mode changed"

log "the storages changed, so the kit run marks the printed envelope stale, once"
grep -q '^Envelope: OUT OF DATE' <<<"$(archiver status)" || die "status does not show the envelope out of date: $(archiver status | grep Envelope)"
# Captured first: healthcheck exits 1 here (the missing extra path logged an ERROR).
HC="$(archiver healthcheck 2>&1)"
grep -q 'Envelope: the printed break-glass envelope is out of date' <<<"$HC" || die "healthcheck does not warn"
[ "$(grep -c 'printed break-glass envelope is out of date' "$LOG")" = 1 ] || die "the stale warning was not logged exactly once"
archiver recovery-kit force >/dev/null 2>&1 || die "second kit run failed"
[ "$(grep -c 'printed break-glass envelope is out of date' "$LOG")" = 1 ] || die "the stale warning repeated"

log "reprinting and confirming clears it"
archiver envelope "$OUT" >/dev/null 2>&1 && archiver envelope confirm >/dev/null 2>&1 || die "reprint failed"
grep -q '^Envelope: current' <<<"$(archiver status)" || die "a reprinted envelope is not current"

log "accented letters reach the PDF intact; characters its fonts lack leave the HTML only"
printf 'p\xc3\xa4ssw\xc3\xb6rd-123' >"$SECRETS_DIR/recovery_password"
archiver envelope "$OUT" >/tmp/env.out 2>&1 || { cat /tmp/env.out; die "envelope with an accented password failed"; }
grep -aq "$(printf '(p\xe4ssw\xf6rd-123)')" "$PDF" || die "the accented password is not in the PDF in its fonts' encoding"
printf 'emoji-\xf0\x9f\x94\x91-pass' >"$SECRETS_DIR/recovery_password"
archiver envelope "$OUT" >/tmp/env.out 2>&1 || { cat /tmp/env.out; die "envelope with an emoji password failed"; }
grep -q 'The PDF was not written' /tmp/env.out || die "no explanation for the missing PDF"
[ ! -e "$PDF" ] || die "a PDF that cannot show the password was left in place"
grep -q "emoji-$(printf '\xf0\x9f\x94\x91')-pass" "$HTML" || die "the HTML page lost the password"
printf 'tab\tpassword-1' >"$SECRETS_DIR/recovery_password"
archiver envelope "$OUT" >/tmp/env.out 2>&1 || die "envelope with a tab in the password failed"
[ ! -e "$PDF" ] || die "a PDF was written for a password with a tab, which it cannot show"

echo "=== ENVELOPE OK: page + credentials per storage, owner-only files, valid PDF, extras in the kit, stale detection once ==="
