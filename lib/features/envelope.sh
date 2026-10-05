#!/bin/bash
# The break-glass envelope: one printable page holding what a person needs to recover with
# no running system and no help from a storage's owner. The recovery password, then for each
# storage target where the recovery kit sits and a credential that can read it, with QR codes,
# the decrypt command and the next steps. Written as HTML and as a PDF (drawn directly, no
# renderer), to a directory the user prints from and then deletes; never sent anywhere.
#
# A target's break-glass credential (STORAGE_TARGET_N_BREAKGLASS_*), when set, goes on the
# page instead of the backup credential; otherwise the backup credential does, marked FULL
# ACCESS. The page's fingerprint (a hash keyed by the recovery password, revealing nothing)
# lets `archiver envelope confirm` record what was printed and status warn when it is stale.

ENVELOPE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${RECOVERY_KIT_FEATURE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"

# What was confirmed printed, and what the page would say now (refreshed with each kit run):
# "<fingerprint> <epoch>". Neither holds a secret.
ENVELOPE_CONFIRMED_FILE="${LOG_DIR}/.envelope-confirmed"
ENVELOPE_CURRENT_FILE="${LOG_DIR}/.envelope-current"
# What `archiver envelope` last wrote: confirming records this, never the configuration as it
# is at confirm time, which may have changed since the page was printed.
ENVELOPE_WRITTEN_FILE="${LOG_DIR}/.envelope-written"

# The page, as records: ENVELOPE_KINDS[i] is one of title, sub, h, p, m, warn, qr, notes,
# rule; ENVELOPE_TEXTS[i] its text (for qr, the encoded data; for notes, a line count).
ENVELOPE_KINDS=()
ENVELOPE_TEXTS=()

envelope_add() { ENVELOPE_KINDS+=("${1}"); ENVELOPE_TEXTS+=("${2:-}"); }

# envelope_add_qr DATA: a QR code of DATA, or, when DATA is too long for one, a note to type
# it from the text instead. Returns 1 in that case. DATA reaches qrencode on stdin.
envelope_add_qr() {
  if printf '%s' "${1}" | qrencode -o /dev/null 2>/dev/null; then
    envelope_add qr "${1}"
  else
    envelope_add p "Too long for a QR code: type it from the text."
    return 1
  fi
}

# envelope_add_lines KIND TEXT: one record per line of a multi-line text.
envelope_add_lines() {
  local line
  while IFS= read -r line || [[ -n "${line}" ]]; do
    envelope_add "${1}" "${line}"
  done <<<"${2}"
}

# envelope_target N: the page block for storage target N.
envelope_target() {
  local n="${1}" v
  local name_var="STORAGE_TARGET_${n}_NAME" type_var="STORAGE_TARGET_${n}_TYPE"
  local name="${!name_var}" type="${!type_var}" kit
  kit="$(recovery_kit_file_name)"
  local role="secondary"
  [[ "${n}" -eq 1 ]] && role="primary"
  envelope_add h "Storage ${n}: ${name} (${type}, ${role})"
  envelope_target_qr "${n}" "${type}"

  # bg NAME: the break-glass value of a credential, else empty.
  bg() { v="STORAGE_TARGET_${n}_BREAKGLASS_${1}"; printf '%s' "${!v:-}"; }
  # cred NAME: the backup credential.
  cred() { v="STORAGE_TARGET_${n}_${1}"; printf '%s' "${!v:-}"; }
  access() {
    if [[ "${1}" == breakglass ]]; then
      envelope_add p "Credential: this storage's break-glass credential (set for reading the kit, not for backups)."
    else
      envelope_add warn "FULL ACCESS: this is the backup credential. Whoever holds this page can also delete the backups on this storage."
    fi
  }

  case "${type}" in
    local)
      v="STORAGE_TARGET_${n}_LOCAL_PATH"
      envelope_add p "A disk attached to the backup host. No credential: whoever has the disk has the data (encrypted)."
      envelope_add m "Path in the container: ${!v}"
      envelope_add m "Kit: ${!v%/}/${kit}"
      ;;
    sftp)
      local url user port path
      v="STORAGE_TARGET_${n}_SFTP_URL"; url="${!v}"
      v="STORAGE_TARGET_${n}_SFTP_USER"; user="${!v}"
      # A break-glass key may belong to its own (read-only) account.
      v="STORAGE_TARGET_${n}_BREAKGLASS_SFTP_USER"
      [[ -n "$(bg SSH_KEY)" && -n "${!v:-}" ]] && user="${!v}"
      v="STORAGE_TARGET_${n}_SFTP_PORT"; port="${!v:-22}"
      v="STORAGE_TARGET_${n}_SFTP_PATH"; path="${!v}"
      envelope_add m "Host: ${url} Port: ${port} User: ${user}"
      envelope_add m "Kit: /${path#/}/${kit}"
      local key mode=breakglass
      key="$(bg SSH_KEY)"
      if [[ -z "${key}" ]]; then
        mode=backup
        [[ -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ]] && key="$(<"${DUPLICACY_SSH_PRIVATE_KEY_FILE}")"
      fi
      access "${mode}"
      if [[ -n "${key}" ]]; then
        envelope_add p "SSH private key (save as ssh_private_key, then chmod 600):"
        envelope_add_lines m "${key}"
        envelope_add m "Fetch: sftp -P ${port} -i ssh_private_key ${user}@${url}:/${path#/}/${kit} ."
      else
        envelope_add warn "No SSH private key is configured, so this storage cannot be reached from this page."
      fi
      ;;
    b2)
      local bucket id key mode=breakglass
      v="STORAGE_TARGET_${n}_B2_BUCKETNAME"; bucket="${!v}"
      id="$(bg B2_ID)"; key="$(bg B2_KEY)"
      if [[ -z "${id}" || -z "${key}" ]]; then mode=backup; id="$(cred B2_ID)"; key="$(cred B2_KEY)"; fi
      envelope_add m "Backblaze B2 bucket: ${bucket}"
      envelope_add m "Kit: ${kit} (bucket root; also downloadable in the B2 web UI)"
      access "${mode}"
      envelope_add m "Key ID: ${id}"
      envelope_add m "Application key: ${key}"
      ;;
    s3)
      local bucket endpoint region id secret mode=breakglass
      v="STORAGE_TARGET_${n}_S3_BUCKETNAME"; bucket="${!v}"
      v="STORAGE_TARGET_${n}_S3_ENDPOINT"; endpoint="${!v}"
      v="STORAGE_TARGET_${n}_S3_REGION"; region="${!v:-none}"
      id="$(bg S3_ID)"; secret="$(bg S3_SECRET)"
      if [[ -z "${id}" || -z "${secret}" ]]; then mode=backup; id="$(cred S3_ID)"; secret="$(cred S3_SECRET)"; fi
      envelope_add m "S3 endpoint: ${endpoint} Region: ${region} Bucket: ${bucket}"
      envelope_add m "Kit: ${kit} (bucket root)"
      access "${mode}"
      envelope_add m "Access key ID: ${id}"
      envelope_add m "Secret key: ${secret}"
      ;;
  esac
}

# envelope_target_qr N TYPE: the QR code for target N's credential, placed first in its
# block so it floats beside the text. The same credential choice as envelope_target.
envelope_target_qr() {
  local n="${1}" v id key data=""
  getv() { v="STORAGE_TARGET_${n}_${1}"; printf '%s' "${!v:-}"; }
  case "${2}" in
    sftp)
      data="$(getv BREAKGLASS_SSH_KEY)"
      [[ -z "${data}" && -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ]] && data="$(<"${DUPLICACY_SSH_PRIVATE_KEY_FILE}")"
      ;;
    b2)
      id="$(getv BREAKGLASS_B2_ID)"; key="$(getv BREAKGLASS_B2_KEY)"
      [[ -z "${id}" || -z "${key}" ]] && { id="$(getv B2_ID)"; key="$(getv B2_KEY)"; }
      data="B2 bucket: $(getv B2_BUCKETNAME)"$'\n'"Key ID: ${id}"$'\n'"Application key: ${key}"
      ;;
    s3)
      id="$(getv BREAKGLASS_S3_ID)"; key="$(getv BREAKGLASS_S3_SECRET)"
      [[ -z "${id}" || -z "${key}" ]] && { id="$(getv S3_ID)"; key="$(getv S3_SECRET)"; }
      data="S3 endpoint: $(getv S3_ENDPOINT)"$'\n'"Bucket: $(getv S3_BUCKETNAME)"$'\n'"Access key ID: ${id}"$'\n'"Secret key: ${key}"
      ;;
  esac
  [[ -n "${data}" ]] && envelope_add_qr "${data}"
  return 0
}

# envelope_build: fill the records from the loaded configuration.
envelope_build() {
  ENVELOPE_KINDS=(); ENVELOPE_TEXTS=()
  local host kit i
  host="$(recovery_kit_host)"
  kit="$(recovery_kit_file_name)"
  envelope_add title "Break-glass envelope: ${host}"
  envelope_add sub ""            # date and fingerprint, filled in by envelope_write
  envelope_add p "Everything needed to recover ${host}'s backups with no running system: the recovery password unlocks the recovery kit, and any ONE storage below holds a copy of the kit and the backups. Keep this page sealed and somewhere that does not share fate with the backups."
  envelope_add h "1. Recovery password"
  local note="Also in the QR code beside it. Type it exactly; it is case-sensitive."
  envelope_add_qr "${RECOVERY_PASSWORD}" || note="Type it exactly; it is case-sensitive."
  envelope_add m "${RECOVERY_PASSWORD}"
  envelope_add p "${note}"
  envelope_add h "2. Get the kit from any one storage, then decrypt it"
  envelope_add m "openssl enc -d -aes-256-cbc -pbkdf2 -in ${kit} | tar -xvf -"
  envelope_add p "Enter the recovery password when asked. The kit holds the whole configuration: archiver.env, secrets/ (every password and key), RECREATE.txt (how this deployment was run) and README.txt. Run the image ghcr.io/sisyphusmd/archiver with that configuration and restore with 'archiver restore'."
  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    envelope_add rule
    envelope_target "${i}"
  done
  envelope_add rule
  envelope_add notes "Account recovery and 2FA codes (storage logins, email), written by hand:|3"
  envelope_add p "After printing: 'archiver envelope confirm', then delete the files. Reprint when 'archiver status' says this envelope is out of date."
}

# envelope_fingerprint: what the page says, hashed with the recovery password as the key. The
# key goes through stdin, never argv; the result reveals nothing about the page's secrets.
envelope_fingerprint() {
  local i
  {
    printf '%s\n' "${RECOVERY_PASSWORD}"
    for i in "${!ENVELOPE_KINDS[@]}"; do
      [[ "${ENVELOPE_KINDS[i]}" == sub ]] && continue
      printf '%s\t%s\n' "${ENVELOPE_KINDS[i]}" "${ENVELOPE_TEXTS[i]}"
    done
  } | sha256sum | cut -c1-16
}

envelope_html_escape() {
  # bash 5.2's patsub_replacement would turn each '&' below back into the matched character.
  shopt -u patsub_replacement 2>/dev/null
  local s="${1}"
  s="${s//&/&amp;}"; s="${s//</&lt;}"; s="${s//>/&gt;}"; s="${s//\"/&quot;}"
  printf '%s' "${s}"
}

# envelope_write_html FILE
envelope_write_html() {
  local out="${1}" i kind text label lines
  {
    cat <<'EOF'
<!doctype html>
<html><head><meta charset="utf-8"><title>Break-glass envelope</title>
<style>
@page { size: letter; margin: 14mm; }
body { font: 10pt/1.35 Helvetica, Arial, sans-serif; color: #000; background: #fff; max-width: 190mm; margin: 0 auto; }
h1 { font-size: 16pt; margin: 0 0 2pt; }
h2 { font-size: 11pt; margin: 10pt 0 3pt; }
.sub { font-size: 8.5pt; color: #333; margin: 0 0 6pt; }
pre { font: 8.5pt/1.25 "Courier New", Courier, monospace; margin: 1pt 0; white-space: pre-wrap; word-break: break-all; }
.warn { font-weight: bold; border: 1.5pt solid #000; padding: 2pt 4pt; margin: 3pt 0; }
.qr { float: right; margin: 0 0 4pt 10pt; }
.qr svg { width: 38mm; height: 38mm; }
h2, hr, .notes { clear: both; }
hr { border: 0; border-top: 0.75pt solid #000; margin: 8pt 0 2pt; }
.notes div { border-bottom: 0.75pt solid #000; height: 7mm; }
</style></head><body>
EOF
    for i in "${!ENVELOPE_KINDS[@]}"; do
      kind="${ENVELOPE_KINDS[i]}"; text="${ENVELOPE_TEXTS[i]}"
      case "${kind}" in
        title) printf '<h1>%s</h1>\n' "$(envelope_html_escape "${text}")" ;;
        sub)   printf '<p class="sub">%s</p>\n' "$(envelope_html_escape "${text}")" ;;
        h)     printf '<h2>%s</h2>\n' "$(envelope_html_escape "${text}")" ;;
        p)     printf '<p>%s</p>\n' "$(envelope_html_escape "${text}")" ;;
        m)     printf '<pre>%s</pre>\n' "$(envelope_html_escape "${text}")" ;;
        warn)  printf '<p class="warn">%s</p>\n' "$(envelope_html_escape "${text}")" ;;
        qr)    printf '<div class="qr">'; printf '%s' "${text}" | qrencode -t SVG -m 1 -o - | sed '1{/^<?xml/d}'; printf '</div>\n' ;;
        rule)  printf '<hr>\n' ;;
        notes)
          label="${text%|*}"; lines="${text##*|}"
          printf '<p class="notes"><b>%s</b></p><div class="notes">' "$(envelope_html_escape "${label}")"
          for ((l = 0; l < lines; l++)); do printf '<div></div>'; done
          printf '</div>\n' ;;
      esac
    done
    printf '</body></html>\n'
  } >"${out}"
}

# envelope_write_pdf FILE: a PDF drawn directly, in the standard fonts PDF readers carry
# built in (Helvetica, Courier), with QR codes as filled squares. No renderer is needed, so
# the page needs nothing beyond qrencode. The fonts' encoding is Windows-1252, which covers
# accented Latin letters; text outside it, or with control characters such as a tab, fails
# (status 2) rather than print a credential that cannot be typed back.
envelope_write_pdf() {
  local out="${1}" i kind text records
  for i in "${!ENVELOPE_TEXTS[@]}"; do
    [[ "${ENVELOPE_KINDS[i]}" != qr && "${ENVELOPE_TEXTS[i]}" == *[[:cntrl:]]* ]] && return 2
  done
  records="$(
  {
    for i in "${!ENVELOPE_KINDS[@]}"; do
      kind="${ENVELOPE_KINDS[i]}"; text="${ENVELOPE_TEXTS[i]}"
      if [[ "${kind}" == qr ]]; then
        # qrencode's ASCII output draws each module as two characters, '#' for dark.
        text="$(printf '%s' "${text}" | qrencode -t ASCII -m 2 -o - | sed 's/##/1/g; s/  /0/g' | paste -sd'|')"
      fi
      printf '%s\t%s\n' "${kind}" "${text}"
    done
  } | iconv -f UTF-8 -t CP1252 2>/dev/null)" || return 2
  printf '%s\n' "${records}" | LC_ALL=C awk -F'\t' -f "${LIB_DIR}/features/envelope-pdf.awk" >"${out}"
}

# envelope_write DIR: build the page and write DIR/envelope-<host>.{html,pdf}. Prints the
# fingerprint.
envelope_write() {
  local dir="${1}" host fp
  envelope_build
  fp="$(envelope_fingerprint)"
  ENVELOPE_TEXTS[1]="Printed $(date '+%Y-%m-%d'). Envelope fingerprint ${fp}; 'archiver status' shows the current one."
  host="$(recovery_kit_host)"
  # Written to private temporary files and moved into place: an existing output file keeps
  # its permissions when rewritten, which could leave the secrets readable.
  (
    umask 077
    mkdir -p "${dir}" || exit 1
    html="$(mktemp "${dir}/.envelope.XXXXXX")" && pdf="$(mktemp "${dir}/.envelope.XXXXXX")" || exit 1
    trap 'rm -f "${html}" "${pdf}"' EXIT
    envelope_write_html "${html}" && mv -f "${html}" "${dir}/envelope-${host}.html" || exit 1
    rc=0; envelope_write_pdf "${pdf}" || rc=$?
    if [[ ${rc} -eq 2 ]]; then
      rm -f "${dir}/envelope-${host}.pdf"
      echo "The PDF was not written: some text on the page (a password or credential) has characters the PDF's fonts cannot show. Print the HTML page instead." >&2
      exit 0
    fi
    [[ ${rc} -eq 0 ]] && mv -f "${pdf}" "${dir}/envelope-${host}.pdf"
  ) || return 1
  printf '%s %s\n' "${fp}" "$(date +%s)" >"${ENVELOPE_WRITTEN_FILE}" || return 1
  printf '%s\n' "${fp}"
}

# A confirmed envelope is due for a fresh print after this long, even if nothing changed: the
# yearly look at the paper catches a lost envelope or a page nobody can read any more.
ENVELOPE_MAX_AGE_DAYS=365
ENVELOPE_NOTIFIED_FILE="${LOG_DIR}/.envelope-notified"

# envelope_state FILE: prints "<fingerprint> <epoch>" from FILE, or nothing.
envelope_state() { [[ -f "${1}" ]] && head -1 "${1}"; }

# envelope_check: record what the page would say now (for status) and, once per change,
# notify when a confirmed envelope no longer matches or is older than ENVELOPE_MAX_AGE_DAYS.
# Nothing is checked until an envelope has been confirmed printed.
envelope_check() {
  [[ -n "${RECOVERY_PASSWORD:-}" ]] || return 0
  envelope_build
  local fp now confirmed cfp cepoch reason="" notified
  fp="$(envelope_fingerprint)" || return 1
  now="$(date +%s)"
  printf '%s %s\n' "${fp}" "${now}" >"${ENVELOPE_CURRENT_FILE}.tmp" && mv "${ENVELOPE_CURRENT_FILE}.tmp" "${ENVELOPE_CURRENT_FILE}" || return 1
  confirmed="$(envelope_state "${ENVELOPE_CONFIRMED_FILE}")"
  [[ -n "${confirmed}" ]] || return 0
  read -r cfp cepoch <<<"${confirmed}"
  if [[ "${cfp}" != "${fp}" ]]; then
    reason="changed:${fp}"
  elif (( now - cepoch > ENVELOPE_MAX_AGE_DAYS * 86400 )); then
    reason="aged:${cepoch}"
  fi
  [[ -n "${reason}" ]] || return 0
  notified="$(cat "${ENVELOPE_NOTIFIED_FILE}" 2>/dev/null)"
  [[ "${notified}" == "${reason}" ]] && return 0
  if [[ "${reason}" == changed:* ]]; then
    log_message "WARNING" "Envelope: the printed break-glass envelope is out of date (a secret or storage on it changed). Print a new one with 'archiver envelope', then run 'archiver envelope confirm'."
    notify "Envelope Out of Date" "The printed break-glass envelope no longer matches the configuration. Print a new one with 'archiver envelope' and confirm it."
  else
    log_message "WARNING" "Envelope: the printed break-glass envelope was confirmed over ${ENVELOPE_MAX_AGE_DAYS} days ago. Check the paper is still there and readable, reprint if needed, and run 'archiver envelope confirm'."
    notify "Envelope Check Due" "The break-glass envelope was confirmed over a year ago. Check it is still there and readable, then run 'archiver envelope confirm'."
  fi
  printf '%s\n' "${reason}" >"${ENVELOPE_NOTIFIED_FILE}"
}

# envelope_confirm: record the last page written as printed now. Returns 2 when no page was
# written, 3 when the configuration has changed since it was (the paper would be stale).
envelope_confirm() {
  local written fp current
  written="$(envelope_state "${ENVELOPE_WRITTEN_FILE}")"
  [[ -n "${written}" ]] || return 2
  fp="${written%% *}"
  envelope_build
  current="$(envelope_fingerprint)" || return 1
  [[ "${current}" == "${fp}" ]] || return 3
  printf '%s %s\n' "${fp}" "$(date +%s)" >"${ENVELOPE_CONFIRMED_FILE}" || return 1
  cp "${ENVELOPE_CONFIRMED_FILE}" "${ENVELOPE_CURRENT_FILE}"
  rm -f "${ENVELOPE_NOTIFIED_FILE}"
  printf '%s\n' "${fp}"
}
