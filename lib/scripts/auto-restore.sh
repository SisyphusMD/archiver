#!/bin/bash
# Non-interactive restore driven by environment variables

AUTO_RESTORE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${DUPLICACY_RESTORE_FEATURE}"

# Simple log_message for user output (auto-restore is non-interactive)
log_message() {
  echo "$@"
}

usage() {
  echo "Usage: archiver auto-restore" >&2
  echo "Required env: SNAPSHOT_ID, LOCAL_DIR" >&2
  echo "Optional env: REVISION (default 'latest'), STORAGE_TARGET (name or id)," >&2
  echo "              OVERWRITE, DELETE_EXTRA, HASH_COMPARE, IGNORE_OWNERSHIP (non-empty enables)," >&2
  echo "              RESTORE_THREADS (default matches DUPLICACY_THREADS)" >&2
  echo "Exit codes: 0=restored, 1=snapshot not found or restore failed," >&2
  echo "            2=infra/unreachable or invalid env, 3=lock held" >&2
  exit 2
}

check_required_env() {
  if [ -z "${SNAPSHOT_ID}" ]; then
    echo "ERROR: SNAPSHOT_ID environment variable is required" >&2
    usage
  fi
  if [ -z "${LOCAL_DIR}" ]; then
    echo "ERROR: LOCAL_DIR environment variable is required" >&2
    usage
  fi
}

check_lock() {
  if is_lock_valid; then
    echo "ERROR: Archiver backup lock is held; cannot run auto-restore" >&2
    exit 3
  fi
}

resolve_restore_options() {
  REVISION="${REVISION:-latest}"
  RESTORE_THREADS="${RESTORE_THREADS:-${DUPLICACY_THREADS}}"
  RESTORE_FLAGS=""

  if [ -n "${HASH_COMPARE}" ]; then
    RESTORE_FLAGS="${RESTORE_FLAGS} -hash"
  fi
  if [ -n "${OVERWRITE}" ]; then
    RESTORE_FLAGS="${RESTORE_FLAGS} -overwrite"
  fi
  if [ -n "${DELETE_EXTRA}" ]; then
    RESTORE_FLAGS="${RESTORE_FLAGS} -delete"
  fi
  if [ -n "${IGNORE_OWNERSHIP}" ]; then
    RESTORE_FLAGS="${RESTORE_FLAGS} -ignore-owner"
  fi
}

# Populate TARGET_ORDER with storage IDs to try, honoring STORAGE_TARGET pin if set.
resolve_target_order() {
  local i

  TARGET_ORDER=()

  if [ -n "${STORAGE_TARGET}" ]; then
    if [[ "${STORAGE_TARGET}" =~ ^[0-9]+$ ]]; then
      resolve_storage_target_by_id "${STORAGE_TARGET}" || \
        { echo "ERROR: STORAGE_TARGET '${STORAGE_TARGET}' is not a valid storage target id" >&2; exit 2; }
    else
      resolve_storage_target_by_name "${STORAGE_TARGET}" || \
        { echo "ERROR: STORAGE_TARGET '${STORAGE_TARGET}' does not match any configured storage name" >&2; exit 2; }
    fi
    TARGET_ORDER=("${SELECTED_STORAGE_TARGET_ID}")
    return
  fi

  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    TARGET_ORDER+=("${i}")
  done
}

prepare_local_dir() {
  if [ ! -d "${LOCAL_DIR}" ]; then
    mkdir -p -m 0755 "${LOCAL_DIR}" || \
      { echo "ERROR: Failed to create '${LOCAL_DIR}'" >&2; exit 2; }
  fi

  cd "${LOCAL_DIR}" || \
    { echo "ERROR: Failed to change directory to '${LOCAL_DIR}'" >&2; exit 2; }
}

# Probes one storage target for SNAPSHOT_ID and resolves REVISION.
# Returns 0=ready to restore, 1=reachable but snapshot missing, 2=unreachable.
attempt_target() {
  local storage_id="${1}"
  local revs

  resolve_storage_target_by_id "${storage_id}"
  echo "Trying '${SELECTED_STORAGE_TARGET_NAME}' (${SELECTED_STORAGE_TARGET_TYPE})..."

  rm -rf .duplicacy

  if ! duplicacy_init_for_restore; then
    echo "  unreachable"
    return 2
  fi

  if ! revs="$(list_snapshot_revisions "${SNAPSHOT_ID}")"; then
    echo "  list failed"
    return 2
  fi

  if [ -z "${revs}" ]; then
    echo "  no revisions"
    return 1
  fi

  if [ "${REVISION}" = "latest" ]; then
    REVISION="$(echo "${revs}" | head -n 1)"
  elif ! echo "${revs}" | grep -qx "${REVISION}"; then
    echo "  revision ${REVISION} not found"
    return 1
  fi

  echo "  using revision ${REVISION}"
  return 0
}

restore_from_targets() {
  local storage_id
  local status
  local unreachable=0
  local attempted=0

  for storage_id in "${TARGET_ORDER[@]}"; do
    attempted=$((attempted + 1))
    attempt_target "${storage_id}"
    status=$?

    if [ "${status}" -eq 0 ]; then
      echo "Restoring from '${SELECTED_STORAGE_TARGET_NAME}' at revision ${REVISION}..."
      if perform_restore; then
        echo "Repository restored."
        exit 0
      fi
      echo "ERROR: Restore from '${SELECTED_STORAGE_TARGET_NAME}' failed" >&2
      exit 1
    elif [ "${status}" -eq 2 ]; then
      unreachable=$((unreachable + 1))
    fi
  done

  if [ "${unreachable}" -eq "${attempted}" ]; then
    echo "ERROR: All storage targets unreachable" >&2
    exit 2
  fi

  echo "ERROR: Snapshot '${SNAPSHOT_ID}' not found on any reachable storage target" >&2
  exit 1
}

main() {
  check_required_env
  check_lock
  count_storage_targets
  verify_target_settings
  check_required_secrets
  resolve_restore_options
  resolve_target_order
  prepare_local_dir
  restore_from_targets
}

main
