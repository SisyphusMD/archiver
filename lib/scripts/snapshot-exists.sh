#!/bin/bash
# Non-interactive snapshot existence probe across all storage targets

SNAPSHOT_EXISTS_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${DUPLICACY_RESTORE_FEATURE}"

# Divert log_message output to stderr; stdout is reserved for the final answer
log_message() {
  echo "$@" >&2
}

usage() {
  echo "Usage: archiver snapshot-exists" >&2
  echo "Required env: SNAPSHOT_ID" >&2
  echo "Exit codes: 0=exists, 1=not found, 2=infra/unreachable, 3=lock held" >&2
  exit 2
}

check_required_env() {
  if [ -z "${SNAPSHOT_ID}" ]; then
    echo "ERROR: SNAPSHOT_ID environment variable is required" >&2
    usage
  fi
}

check_lock() {
  if is_lock_valid; then
    echo "ERROR: Archiver backup lock is held; cannot check snapshots" >&2
    exit 3
  fi
}

# Probes one storage target for SNAPSHOT_ID.
# Returns 0=found, 1=reachable but no revisions, 2=unreachable.
probe_target() {
  local storage_id="${1}"
  local workdir
  local orig_dir="${PWD}"
  local revs
  local rev_count

  resolve_storage_target_by_id "${storage_id}"
  echo "Checking '${SELECTED_STORAGE_TARGET_NAME}' (${SELECTED_STORAGE_TARGET_TYPE})..." >&2

  workdir="$(mktemp -d)"
  cd "${workdir}" || { rm -rf "${workdir}"; return 2; }

  if ! duplicacy_init_for_restore >&2; then
    echo "  unreachable" >&2
    cd "${orig_dir}"
    rm -rf "${workdir}"
    return 2
  fi

  if ! revs="$(list_snapshot_revisions "${SNAPSHOT_ID}")"; then
    echo "  list failed" >&2
    cd "${orig_dir}"
    rm -rf "${workdir}"
    return 2
  fi

  cd "${orig_dir}"
  rm -rf "${workdir}"

  if [ -n "${revs}" ]; then
    rev_count="$(echo "${revs}" | wc -l | tr -d ' ')"
    echo "  found ${rev_count} revision(s)" >&2
    return 0
  fi

  echo "  no revisions" >&2
  return 1
}

probe_all_targets() {
  local storage_id
  local status
  local found=0
  local reachable=0

  for storage_id in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    probe_target "${storage_id}"
    status=$?
    if [ "${status}" -eq 0 ]; then
      found=1
      break
    elif [ "${status}" -eq 1 ]; then
      reachable=$((reachable + 1))
    fi
  done

  if [ "${found}" -eq 1 ]; then
    echo "EXISTS"
    exit 0
  fi

  if [ "${reachable}" -eq 0 ]; then
    echo "UNDETERMINED"
    exit 2
  fi

  echo "NOT FOUND"
  exit 1
}

main() {
  check_required_env
  check_lock
  count_storage_targets
  verify_target_settings
  check_required_secrets
  probe_all_targets
}

main
