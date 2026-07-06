#!/bin/bash
# Duplicacy restore operations: storage target resolution, repo init, revision listing, restore

DUPLICACY_RESTORE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"

# Resolve storage target by numeric ID. Sets SELECTED_STORAGE_TARGET_{ID,NAME,TYPE}.
resolve_storage_target_by_id() {
  local id="${1}"

  if ! [[ "${id}" =~ ^[0-9]+$ ]] || [ "${id}" -lt 1 ] || [ "${id}" -gt "${STORAGE_TARGET_COUNT}" ]; then
    return 1
  fi

  local storage_name_var="STORAGE_TARGET_${id}_NAME"
  local storage_type_var="STORAGE_TARGET_${id}_TYPE"

  SELECTED_STORAGE_TARGET_ID="${id}"
  SELECTED_STORAGE_TARGET_NAME="${!storage_name_var}"
  SELECTED_STORAGE_TARGET_TYPE="${!storage_type_var}"
}

# Resolve storage target by its configured name. Sets SELECTED_STORAGE_TARGET_{ID,NAME,TYPE}.
resolve_storage_target_by_name() {
  local name="${1}"
  local i

  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    local storage_name_var="STORAGE_TARGET_${i}_NAME"
    if [[ "${!storage_name_var}" == "${name}" ]]; then
      resolve_storage_target_by_id "${i}"
      return 0
    fi
  done
  return 1
}

# Initialize duplicacy in the current working directory for the currently selected
# storage target. Requires SELECTED_STORAGE_TARGET_{ID,NAME,TYPE} and SNAPSHOT_ID set.
duplicacy_init_for_restore() {
  local storage_id="${SELECTED_STORAGE_TARGET_ID}"
  # Sanitize (e.g. hyphens -> _) so the -storage-name matches what duplicacy-backup.sh
  # registered and the DUPLICACY_<NAME>_* env vars stay valid shell identifiers.
  local storage_name
  storage_name="$(sanitize_storage_name "${SELECTED_STORAGE_TARGET_NAME}")"
  local storage_type="${SELECTED_STORAGE_TARGET_TYPE}"
  local storage_url

  # Export the DUPLICACY_<NAME>_* credentials the duplicacy binary reads.
  export_duplicacy_storage_secrets "${storage_id}"

  # sftp needs its key files present before init even tries to reach the host.
  if [[ "${storage_type}" == "sftp" ]]; then
    if [ ! -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ] || [ ! -f "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" ]; then
      handle_error "Missing SSH key files for SFTP storage '${storage_name}'."
      return 1
    fi
  fi

  storage_url="$(build_storage_url "${storage_id}")"
  if [[ -z "${storage_url}" ]]; then
    handle_error "'${storage_type}' is not a supported storage type."
    return 1
  fi

  duplicacy init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
    -storage-name "${storage_name}" "${SNAPSHOT_ID}" \
    "${storage_url}" || \
    { handle_error "Duplicacy ${storage_type} storage initialization failed for '${storage_name}'."; return 1; }

  # Persist type-specific credentials to the storage's preferences file.
  case "${storage_type}" in
    sftp)
      duplicacy set -storage "${storage_name}" -key ssh_key_file -value "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" || \
        { handle_error "Setting the Duplicacy SFTP key file failed for '${storage_name}'."; return 1; }
      ;;
    b2)
      local config_b2_id_var="STORAGE_TARGET_${storage_id}_B2_ID"
      local config_b2_key_var="STORAGE_TARGET_${storage_id}_B2_KEY"

      duplicacy set -storage "${storage_name}" -key b2_id -value "${!config_b2_id_var}" || \
        { handle_error "Setting the Duplicacy B2 keyID failed for '${storage_name}'."; return 1; }

      duplicacy set -storage "${storage_name}" -key b2_key -value "${!config_b2_key_var}" || \
        { handle_error "Setting the Duplicacy B2 applicationKey failed for '${storage_name}'."; return 1; }
      ;;
    s3)
      local config_s3_id_var="STORAGE_TARGET_${storage_id}_S3_ID"
      local config_s3_secret_var="STORAGE_TARGET_${storage_id}_S3_SECRET"

      duplicacy set -storage "${storage_name}" -key s3_id -value "${!config_s3_id_var}" || \
        { handle_error "Setting the Duplicacy S3 ID failed for '${storage_name}'."; return 1; }

      duplicacy set -storage "${storage_name}" -key s3_secret -value "${!config_s3_secret_var}" || \
        { handle_error "Setting the Duplicacy S3 Secret failed for '${storage_name}'."; return 1; }
      ;;
  esac

  duplicacy set -storage "${storage_name}" -key password -value "${STORAGE_PASSWORD}" || \
    { handle_error "Setting the Duplicacy storage password failed for '${storage_name}'."; return 1; }

  duplicacy set -storage "${storage_name}" -key rsa_passphrase -value "${RSA_PASSPHRASE}" || \
    { handle_error "Setting the Duplicacy RSA Passphrase failed for '${storage_name}'."; return 1; }
}

# Echo revision numbers for SNAPSHOT_ID on stdout (highest first, one per line).
# Requires duplicacy to have been init'd in CWD. Returns 0 even if zero revisions.
list_snapshot_revisions() {
  local snapshot_id="${1}"
  local list_output
  local exit_status

  list_output="$(duplicacy list -id "${snapshot_id}" 2>&1)"
  exit_status=$?

  if [ "${exit_status}" -ne 0 ]; then
    echo "${list_output}" >&2
    return "${exit_status}"
  fi

  echo "${list_output}" | awk -v id="${snapshot_id}" \
    '$1 == "Snapshot" && $2 == id && $3 == "revision" { print $4 }' | sort -rn
}

# Echo highest revision number, or empty string if none. Returns 0 if found,
# 1 if zero revisions, 2 if listing failed.
resolve_latest_revision() {
  local snapshot_id="${1}"
  local revs
  local latest

  revs="$(list_snapshot_revisions "${snapshot_id}")" || return 2

  latest="$(echo "${revs}" | head -n 1)"
  if [ -z "${latest}" ]; then
    return 1
  fi

  echo "${latest}"
}

# Perform duplicacy restore. Requires CWD to be the restore destination, duplicacy
# initialized, and REVISION / RESTORE_FLAGS / RESTORE_THREADS set.
perform_restore() {
  # shellcheck disable=SC2086
  duplicacy restore -r "${REVISION}" -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" \
    -stats -threads "${RESTORE_THREADS}" ${RESTORE_FLAGS}
}
