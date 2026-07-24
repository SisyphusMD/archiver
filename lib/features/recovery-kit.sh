#!/bin/bash
# Automatic recovery kit: serialize the full effective configuration (non-secret settings +
# every secret + the keys), plus recreation notes and any deployment manifests mounted at
# ${DEPLOYMENT_DIR}, into a single encrypted tar, and place it as a PLAIN file beside the
# duplicacy data on every storage target. Recovery needs only one reachable storage location
# (even via a provider web UI) plus the single recovery password — no other archiver state.
# The kit is write-only: nothing at runtime ever reads it back, so unlike the bundle it is
# never a boot dependency.
#
# Enabled by the presence of the recovery_password secret (file-only, like every secret).
# The kit is re-uploaded only when its content changes, when a target is missing from the
# recorded state, or on `archiver recovery-kit force`.

RECOVERY_KIT_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"
source_if_not_sourced "${CONFIG_SERIALIZE_CORE}"

# Uploaded-state record: line 1 is the kit fingerprint, following lines are the storage
# names that already hold that fingerprint. Lives in LOG_DIR so it survives container
# recreation when logs are host-mounted; if it does not survive, the only cost is one
# redundant re-upload.
RECOVERY_KIT_STATE_FILE="${LOG_DIR}/.recovery-kit-state"

# Placement-scheme version, folded into the uploaded-state fingerprint (see run_recovery_kit).
# Bump it whenever the way the kit is written to a target changes — e.g. its ownership/permission
# handling — so every existing deployment re-stamps its already-placed kit exactly once on the
# first run after upgrade, even when the kit content itself is unchanged.
RECOVERY_KIT_STATE_VERSION="3"

recovery_kit_configured() { [[ -n "${RECOVERY_PASSWORD}" ]]; }

recovery_kit_file_name()   { printf 'archiver-recovery-kit-%s.tar.enc'   "$(hostname)"; }
recovery_kit_readme_name() { printf 'archiver-recovery-kit-%s.README.txt' "$(hostname)"; }

validate_recovery_password() {
  if (( ${#RECOVERY_PASSWORD} < 8 )); then
    handle_error "RECOVERY_PASSWORD must be at least 8 characters; use a long generated password (it is the only thing protecting the recovery kit at rest)."
    return 1
  fi
  if [[ "${RECOVERY_PASSWORD}" == "${STORAGE_PASSWORD}" ]]; then
    handle_error "RECOVERY_PASSWORD must differ from STORAGE_PASSWORD: the recovery kit contains the storage password, so protecting it with the same value defeats the kit."
    return 1
  fi
  return 0
}

# The kit must capture the configuration AS PROVIDED, not the process's runtime state:
# the maintenance/notification checks inject runtime defaults (PRUNE_KEEP, CHECK_BACKUPS,
# NOTIFICATION_SERVICE normalization) — serializing those would make the fingerprint (and
# the kit's config) depend on which command happened to run last. Snapshot the pristine
# values when this file is sourced, which both consumers (main.sh and the recovery-kit
# script) do immediately after config-loader finishes and before any mutation.
declare -gA RECOVERY_KIT_CONFIG_SNAPSHOT=()
recovery_kit_snapshot_config() {
  local v
  RECOVERY_KIT_CONFIG_SNAPSHOT=()
  while IFS= read -r v; do
    if [[ "${v}" == "SERVICE_DIRECTORIES" ]]; then
      RECOVERY_KIT_CONFIG_SNAPSHOT["${v}"]="$(service_directories_scalar)"
    else
      RECOVERY_KIT_CONFIG_SNAPSHOT["${v}"]="${!v}"
    fi
  done < <(compgen -v | grep -E "${CONFIG_NONSECRET_VARS_RE}"; compgen -v | grep -E "${CONFIG_SECRET_VARS_RE}")
}

# Deterministic recreation notes, generated from the snapshotted config (no timestamps —
# the content participates in the change fingerprint). This is what makes the kit stand
# alone when the user did NOT mount their manifest: it lists every fact archiver can know
# about how the container must be put together.
write_recovery_kit_recreate_notes() {
  local out="${1}" n name_var type_var path_var
  {
    echo "How to recreate this Archiver deployment (generated from the running container)"
    echo "================================================================================"
    echo
    echo "1. Load archiver.env as environment variables (compose 'env_file:', or a"
    echo "   Kubernetes ConfigMap via --from-env-file) and mount the secrets/ files under"
    echo "   /run/secrets. Template: https://github.com/SisyphusMD/archiver/blob/main/compose.yaml"
    if compgen -G "${DEPLOYMENT_DIR}/*" >/dev/null; then
      echo
      echo "2. deployment/ in this kit holds the manifest files that were mounted at"
      echo "   ${DEPLOYMENT_DIR} — your actual compose/nix/k8s definition. Prefer those."
    fi
    echo
    echo "Facts this deployment depended on:"
    echo "  - hostname: $(hostname)   (keep it: snapshot IDs and this kit's filename derive from it)"
    [[ -n "${BACKUP_SCHEDULE:-}" ]] && echo "  - BACKUP_SCHEDULE: ${BACKUP_SCHEDULE}"
    [[ -n "${MAINTENANCE_SCHEDULE:-}" ]] && echo "  - MAINTENANCE_SCHEDULE: ${MAINTENANCE_SCHEDULE}"
    [[ -n "${TZ:-}" ]] && echo "  - TZ: ${TZ}"
    echo "  - container paths that need host mounts:"
    local IFS=':'
    for p in ${RECOVERY_KIT_CONFIG_SNAPSHOT[SERVICE_DIRECTORIES]}; do
      [[ -n "${p}" ]] && echo "      ${p}   (data to back up)"
    done
    unset IFS
    n=1
    while :; do
      name_var="STORAGE_TARGET_${n}_NAME"
      [[ -z "${RECOVERY_KIT_CONFIG_SNAPSHOT[${name_var}]:-}" ]] && break
      type_var="STORAGE_TARGET_${n}_TYPE"
      if [[ "${RECOVERY_KIT_CONFIG_SNAPSHOT[${type_var}]:-}" == "local" ]]; then
        path_var="STORAGE_TARGET_${n}_LOCAL_PATH"
        echo "      ${RECOVERY_KIT_CONFIG_SNAPSHOT[${path_var}]:-}   (local storage target '${RECOVERY_KIT_CONFIG_SNAPSHOT[${name_var}]}')"
      fi
      n=$((n + 1))
    done
    if [[ -S /var/run/docker.sock ]]; then
      echo "  - /var/run/docker.sock was mounted (service pre/post hooks use docker)"
    fi
    echo "  - capabilities: cap_drop ALL; cap_add DAC_OVERRIDE (backup);"
    echo "    plus CHOWN + FOWNER to restore files under their original ownership"
  } >"${out}"
}

# Serialize the snapshotted config (plus recreation notes and any mounted deployment
# manifests) into workdir and print its fingerprint: a hash over every file's path +
# content, so any changed, added, renamed, or removed value changes it.
build_recovery_kit_payload() {
  local workdir="${1}"
  (
    local v
    while IFS= read -r v; do
      [[ -v "RECOVERY_KIT_CONFIG_SNAPSHOT[${v}]" ]] || unset "${v}"
    done < <(compgen -v | grep -E "${CONFIG_NONSECRET_VARS_RE}"; compgen -v | grep -E "${CONFIG_SECRET_VARS_RE}")
    for v in "${!RECOVERY_KIT_CONFIG_SNAPSHOT[@]}"; do
      unset "${v}"                                 # drop a possible array form before re-setting as scalar
      printf -v "${v}" '%s' "${RECOVERY_KIT_CONFIG_SNAPSHOT[${v}]}"
    done
    serialize_env_and_secrets "${workdir}/archiver.env" "${workdir}/secrets"
  ) || return 1
  write_recovery_kit_recreate_notes "${workdir}/RECREATE.txt"
  if [[ -d "${DEPLOYMENT_DIR}" ]]; then
    mkdir -p "${workdir}/deployment"
    # Copy only the visible top-level entries, dereferencing symlinks (-L): single-file
    # binds and Kubernetes ConfigMap volumes both present the real content behind links,
    # and a ConfigMap's hidden ..data/..timestamp dirs would otherwise be captured too,
    # duplicating every file inside the kit.
    local entry
    for entry in "${DEPLOYMENT_DIR}"/*; do
      [[ -e "${entry}" ]] || continue
      cp -RL "${entry}" "${workdir}/deployment/" || return 1
    done
    chmod -R u+rwX,go-rwx "${workdir}/deployment"
    rmdir "${workdir}/deployment" 2>/dev/null || true   # drop it when nothing visible was mounted
  fi
  (cd "${workdir}" && find . -type f | LC_ALL=C sort | xargs sha256sum | sha256sum | cut -d' ' -f1)
}

# Encrypt the payload dir into a single kit file. -pass fd:3 keeps the password off
# openssl's argv (visible in /proc while it runs); the same derivation lets a stock
# `openssl enc -d -aes-256-cbc -pbkdf2` recover it anywhere with just the password.
encrypt_recovery_kit() {
  local workdir="${1}" out="${2}"
  local members=(archiver.env secrets RECREATE.txt)
  [[ -d "${workdir}/deployment" ]] && members+=(deployment)
  (
    set -o pipefail
    tar -C "${workdir}" -cf - "${members[@]}" \
      | openssl enc -aes-256-cbc -pbkdf2 -pass fd:3 -out "${out}" 3<<<"${RECOVERY_PASSWORD}"
  ) || return 1
  # No chmod here: the kit is already encrypted and lives inside the datastore, so its outer-file
  # ownership/permissions are stamped at placement time to match the storage target's own
  # duplicacy files rather than being a locked-down root:600 outlier — see recovery_kit_upload_*.
}

write_recovery_kit_readme() {
  local out="${1}" host
  host="$(hostname)"
  cat >"${out}" <<EOF
This is the automatic recovery kit for the Archiver deployment on host '${host}'.

$(recovery_kit_file_name) is an encrypted snapshot of everything needed to recreate the
deployment: archiver.env (the non-secret settings), secrets/ (every secret and key file),
RECREATE.txt (recreation notes), and deployment/ (the deployment manifests, if they were
mounted). It is refreshed automatically whenever any of that changes.

To recover: download the .tar.enc file to any machine and run (it prompts for the recovery
password, which was displayed at setup and belongs in your password manager):

  openssl enc -d -aes-256-cbc -pbkdf2 -in $(recovery_kit_file_name) | tar -xvf -

Then start with RECREATE.txt, or see the 'Configuration Sources' section of the README:
https://github.com/SisyphusMD/archiver
EOF
}

# Minimal JSON string-field extractor for the B2 API (the image ships no jq). Returns the
# FIRST occurrence of "field":"value" in the (flattened) document.
recovery_kit_json_field() {
  local json="${1}" field="${2}"
  printf '%s' "${json}" | tr -d '\n' \
    | grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
    | head -1 | sed 's/.*:[[:space:]]*"\([^"]*\)"$/\1/'
}

recovery_kit_upload_local() {
  local storage_id="${1}" kit="${2}" readme="${3}"
  local path_var="STORAGE_TARGET_${storage_id}_LOCAL_PATH"
  local dest="${!path_var}"
  local kit_dst="${dest}/$(recovery_kit_file_name)"
  local readme_dst="${dest}/$(recovery_kit_readme_name)"
  cp -f "${kit}" "${kit_dst}" || return 1
  cp -f "${readme}" "${readme_dst}" || return 1
  # Match the storage's own duplicacy files (owner:group and mode of its 'config') instead of
  # forcing root:600: the kit is already encrypted and sits beside the chunks, so it should share
  # their access model. Best-effort — a perms mismatch must not fail an otherwise-good upload.
  local ref="${dest}/config"
  if [[ -e "${ref}" ]]; then
    chown "$(stat -c '%u:%g' "${ref}")" "${kit_dst}" "${readme_dst}" \
      && chmod "$(stat -c '%a' "${ref}")" "${kit_dst}" "${readme_dst}" \
      || log_message "WARNING" "Recovery kit uploaded to '${dest}' but could not match perms of ${ref}."
  fi
}

# Convert a 10-char ls-style permission string (e.g. -rwxr-xr-x) to a plain octal mode (e.g.
# 755). Set-uid/gid/sticky high bits are intentionally dropped — they have no business on the
# kit files. Prints nothing on malformed input so callers can fall back.
symbolic_mode_to_octal() {
  local perms="${1#?}"                    # drop the leading file-type character
  [[ ${#perms} -eq 9 ]] || return 0
  local out=0 i base
  for i in 0 3 6; do
    base=0
    [[ "${perms:i:1}"   == r ]] && base=$((base + 4))
    [[ "${perms:i+1:1}" == w ]] && base=$((base + 2))
    case "${perms:i+2:1}" in x|s|t) base=$((base + 1)) ;; esac
    out=$((out * 8 + base))
  done
  printf '%o' "${out}"
}

# Octal mode of a remote file, read over sftp (ls -l -> symbolic -> octal). The perms column is
# formatted by the local sftp client from the returned attributes, so its shape is server-
# independent. Falls back to 644 (readable by a mirror/backup user) when the reference cannot be
# read or parsed, so the kit is never left an owner-only outlier. Extra args are sftp options.
recovery_kit_sftp_ref_mode() {
  local ref="${1}" target="${2}"; shift 2
  local listing perms mode
  listing="$(sftp "$@" -b - "${target}" 2>/dev/null <<EOF
ls -l ${ref}
EOF
  )"
  perms="$(printf '%s\n' "${listing}" | grep -m1 -oE '^[-dbclps][-rwxsStT]{9}')"
  mode="$(symbolic_mode_to_octal "${perms}")"
  # An empty parse or a bare 0 means the server reported no usable mode (e.g. an object-store
  # sftp gateway): fall back to 644 rather than stamping the kit unreadable.
  [[ -n "${mode}" && "${mode}" != 0 ]] || mode=644
  printf '%s' "${mode}"
}

recovery_kit_upload_sftp() {
  local storage_id="${1}" kit="${2}" readme="${3}"
  local user_var="STORAGE_TARGET_${storage_id}_SFTP_USER"
  local url_var="STORAGE_TARGET_${storage_id}_SFTP_URL"
  local port_var="STORAGE_TARGET_${storage_id}_SFTP_PORT"
  local path_var="STORAGE_TARGET_${storage_id}_SFTP_PATH"
  # build_storage_url always emits sftp://user@host:port//path, i.e. an absolute path —
  # match that so the kit lands beside the duplicacy chunks.
  local remote_dir="/${!path_var}"
  local kit_name readme_name
  kit_name="$(recovery_kit_file_name)"
  readme_name="$(recovery_kit_readme_name)"
  local sftp_opts=(-q -P "${!port_var}" -i "${DUPLICACY_SSH_PRIVATE_KEY_FILE}"
    -o BatchMode=yes -o StrictHostKeyChecking=accept-new)
  local target="${!user_var}@${!url_var}"

  # Stamp the kit with the mode duplicacy's own files carry on this storage (read from its
  # 'config'), matching recovery_kit_upload_local — rather than leaving it at the connecting
  # user's umask, which can be owner-only and lock a mirror/backup user out. The kit is already
  # encrypted, so its outer-file mode should mirror the chunks, not be a locked-down outlier.
  local mode
  mode="$(recovery_kit_sftp_ref_mode "${remote_dir}/config" "${target}" "${sftp_opts[@]}")"

  # -b - aborts on the first failed transfer and exits non-zero; accept-new is the same
  # trust-on-first-use posture duplicacy itself has toward this host. The puts are strict; the
  # chmods are best-effort (leading '-') so a server that forbids SETSTAT is still a good upload.
  sftp "${sftp_opts[@]}" -b - "${target}" >/dev/null <<EOF
put ${kit} ${remote_dir}/${kit_name}
put ${readme} ${remote_dir}/${readme_name}
-chmod ${mode} ${remote_dir}/${kit_name}
-chmod ${mode} ${remote_dir}/${readme_name}
EOF
}

# Native B2 API via curl (duplicacy cannot upload arbitrary files). Credentials and auth
# tokens are passed through curl's config-from-stdin (-K -), never argv, so they are not
# visible in /proc while curl runs.
recovery_kit_upload_b2() {
  local storage_id="${1}" kit="${2}" readme="${3}"
  local id_var="STORAGE_TARGET_${storage_id}_B2_ID"
  local key_var="STORAGE_TARGET_${storage_id}_B2_KEY"
  local bucket_var="STORAGE_TARGET_${storage_id}_B2_BUCKETNAME"
  local bucket="${!bucket_var}"
  local curl_opts=(-sSf --connect-timeout 15 --max-time 300)

  local auth api_url token account_id
  auth="$(curl "${curl_opts[@]}" --retry 3 --retry-all-errors -K - <<EOF
user = "${!id_var}:${!key_var}"
url = "https://api.backblazeb2.com/b2api/v2/b2_authorize_account"
EOF
  )" || { log_message "WARNING" "B2 authorization failed for recovery-kit upload."; return 1; }
  api_url="$(recovery_kit_json_field "${auth}" "apiUrl")"
  token="$(recovery_kit_json_field "${auth}" "authorizationToken")"
  account_id="$(recovery_kit_json_field "${auth}" "accountId")"

  # A bucket-restricted application key names its bucket in the auth response; an
  # unrestricted key must look the bucket id up by name.
  local bucket_id=""
  if [[ "$(recovery_kit_json_field "${auth}" "bucketName")" == "${bucket}" ]]; then
    bucket_id="$(recovery_kit_json_field "${auth}" "bucketId")"
  fi
  if [[ -z "${bucket_id}" ]]; then
    local buckets
    buckets="$(curl "${curl_opts[@]}" -K - --data "{\"accountId\":\"${account_id}\",\"bucketName\":\"${bucket}\"}" <<EOF
header = "Authorization: ${token}"
url = "${api_url}/b2api/v2/b2_list_buckets"
EOF
    )" || { log_message "WARNING" "B2 bucket lookup failed for recovery-kit upload."; return 1; }
    bucket_id="$(recovery_kit_json_field "${buckets}" "bucketId")"
  fi
  [[ -n "${bucket_id}" ]] || { log_message "WARNING" "Could not resolve B2 bucket id for '${bucket}'."; return 1; }

  local f name upload upload_url upload_token sha1
  for f in "${kit}" "${readme}"; do
    if [[ "${f}" == "${kit}" ]]; then name="$(recovery_kit_file_name)"; else name="$(recovery_kit_readme_name)"; fi
    upload="$(curl "${curl_opts[@]}" -K - --data "{\"bucketId\":\"${bucket_id}\"}" <<EOF
header = "Authorization: ${token}"
url = "${api_url}/b2api/v2/b2_get_upload_url"
EOF
    )" || { log_message "WARNING" "B2 get_upload_url failed for recovery-kit upload."; return 1; }
    upload_url="$(recovery_kit_json_field "${upload}" "uploadUrl")"
    upload_token="$(recovery_kit_json_field "${upload}" "authorizationToken")"
    sha1="$(sha1sum "${f}" | cut -d' ' -f1)"
    curl "${curl_opts[@]}" -K - --data-binary "@${f}" -o /dev/null <<EOF
header = "Authorization: ${upload_token}"
header = "X-Bz-File-Name: ${name}"
header = "Content-Type: b2/x-auto"
header = "X-Bz-Content-Sha1: ${sha1}"
url = "${upload_url}"
EOF
    [[ $? -eq 0 ]] || { log_message "WARNING" "B2 upload of ${name} failed."; return 1; }
  done
}

# S3-compatible upload via curl's built-in SigV4 signing, path-style addressing (works on
# AWS and MinIO alike, no virtual-host DNS requirements). Credentials go via -K -, not argv.
recovery_kit_upload_s3() {
  local storage_id="${1}" kit="${2}" readme="${3}"
  local id_var="STORAGE_TARGET_${storage_id}_S3_ID"
  local secret_var="STORAGE_TARGET_${storage_id}_S3_SECRET"
  local endpoint_var="STORAGE_TARGET_${storage_id}_S3_ENDPOINT"
  local bucket_var="STORAGE_TARGET_${storage_id}_S3_BUCKETNAME"
  local region_var="STORAGE_TARGET_${storage_id}_S3_REGION"
  local region="${!region_var}"
  # duplicacy uses "none" for region-less endpoints (e.g. MinIO); SigV4 needs some region
  # string and such endpoints ignore it.
  [[ -z "${region}" || "${region}" == "none" ]] && region="us-east-1"

  local f name
  for f in "${kit}" "${readme}"; do
    if [[ "${f}" == "${kit}" ]]; then name="$(recovery_kit_file_name)"; else name="$(recovery_kit_readme_name)"; fi
    curl -sSf --connect-timeout 15 --max-time 300 -K - -T "${f}" \
      "https://${!endpoint_var}/${!bucket_var}/${name}" <<EOF
user = "${!id_var}:${!secret_var}"
aws-sigv4 = "aws:amz:${region}:s3"
EOF
    [[ $? -eq 0 ]] || { log_message "WARNING" "S3 upload of ${name} failed."; return 1; }
  done
}

recovery_kit_snapshot_config

recovery_kit_upload_to_target() {
  local storage_id="${1}" kit="${2}" readme="${3}"
  local type_var="STORAGE_TARGET_${storage_id}_TYPE"
  case "${!type_var}" in
    local) recovery_kit_upload_local "${storage_id}" "${kit}" "${readme}" ;;
    sftp)  recovery_kit_upload_sftp  "${storage_id}" "${kit}" "${readme}" ;;
    b2)    recovery_kit_upload_b2    "${storage_id}" "${kit}" "${readme}" ;;
    s3)    recovery_kit_upload_s3    "${storage_id}" "${kit}" "${readme}" ;;
    *)     return 1 ;;
  esac
}

# Main entry. $1 = "force" to re-upload everywhere regardless of recorded state.
# Returns non-zero (after handle_error) when any target failed; a failed target is left
# out of the state record so the next run retries it automatically.
run_recovery_kit() {
  local force="${1:-}"

  if ! recovery_kit_configured; then
    log_message "INFO" "Recovery kit not configured (no recovery_password secret); skipping."
    return 0
  fi
  validate_recovery_password || return 1

  local workdir
  workdir="$(mktemp -d /tmp/archiver-recovery-kit.XXXXXX)" || { handle_error "Recovery kit: mktemp failed."; return 1; }
  chmod 700 "${workdir}"

  local fingerprint
  fingerprint="$(build_recovery_kit_payload "${workdir}")" || {
    rm -rf "${workdir}"; handle_error "Recovery kit: payload serialization failed."; return 1
  }
  # Version-tag the fingerprint so a placement-scheme change invalidates the recorded state and
  # forces a one-time re-stamp on upgrade, independent of whether the kit content changed.
  fingerprint="v${RECOVERY_KIT_STATE_VERSION}:${fingerprint}"

  local state_hash="" state_names=""
  if [[ -f "${RECOVERY_KIT_STATE_FILE}" ]]; then
    state_hash="$(head -1 "${RECOVERY_KIT_STATE_FILE}")"
    state_names="$(tail -n +2 "${RECOVERY_KIT_STATE_FILE}")"
  fi
  [[ "${state_hash}" != "${fingerprint}" ]] && state_names=""

  local i pending=() done_names=() name name_var
  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    name_var="STORAGE_TARGET_${i}_NAME"
    name="$(sanitize_storage_name "${!name_var}")"
    if [[ "${force}" != "force" ]] && grep -qxF "${name}" <<<"${state_names}"; then
      done_names+=("${name}")
    else
      pending+=("${i}")
    fi
  done

  if [[ ${#pending[@]} -eq 0 ]]; then
    log_message "INFO" "Recovery kit is up to date on all storage targets."
    rm -rf "${workdir}"
    return 0
  fi

  local kit="${workdir}/$(recovery_kit_file_name)" readme="${workdir}/$(recovery_kit_readme_name)"
  encrypt_recovery_kit "${workdir}" "${kit}" || {
    rm -rf "${workdir}"; handle_error "Recovery kit: encryption failed."; return 1
  }
  write_recovery_kit_readme "${readme}"

  local failures=0
  for i in "${pending[@]}"; do
    name_var="STORAGE_TARGET_${i}_NAME"
    name="$(sanitize_storage_name "${!name_var}")"
    if recovery_kit_upload_to_target "${i}" "${kit}" "${readme}"; then
      log_message "INFO" "Recovery kit updated on storage '${name}' ($(recovery_kit_file_name))."
      done_names+=("${name}")
    else
      handle_error "Recovery-kit upload to storage '${name}' failed; will retry on the next run."
      failures=$((failures + 1))
    fi
  done

  {
    printf '%s\n' "${fingerprint}"
    printf '%s\n' "${done_names[@]}"
  } >"${RECOVERY_KIT_STATE_FILE}"
  chmod 600 "${RECOVERY_KIT_STATE_FILE}"

  rm -rf "${workdir}"
  [[ ${failures} -eq 0 ]]
}
