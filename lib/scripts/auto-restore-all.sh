#!/bin/bash
# Non-interactive disaster-recovery restore of EVERY configured service in one
# pass. Iterates SERVICE_DIRECTORIES and runs auto-restore (latest revision,
# trying all storage targets) for each. Built for rebuilding a blank host (e.g.
# the humblepixels vps up.sh tier-3 path). The per-service restore mechanics live
# in auto-restore.sh; this only orchestrates the loop and aggregates the result.

AUTO_RESTORE_ALL_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"

# Non-interactive: plain stdout for progress, a final summary line per service.
log_message() {
  echo "$@"
}

usage() {
  echo "Usage: archiver auto-restore-all" >&2
  echo "Restores every service in SERVICE_DIRECTORIES at its latest revision." >&2
  echo "Non-interactive. Optional env is passed through to each auto-restore:" >&2
  echo "  REVISION, STORAGE_TARGET, OVERWRITE, DELETE_EXTRA, HASH_COMPARE, IGNORE_OWNERSHIP" >&2
  echo "Exit codes: 0=all restored, 1=one or more failed, 2=invalid usage, 3=lock held" >&2
  exit 2
}

check_lock() {
  if is_lock_valid; then
    echo "ERROR: Archiver backup lock is held; cannot run auto-restore-all" >&2
    exit 3
  fi
}

main() {
  [ "$#" -eq 0 ] || usage
  check_lock
  expand_service_directories

  if [ "${#EXPANDED_SERVICE_DIRECTORIES[@]}" -eq 0 ]; then
    echo "ERROR: No service directories resolved from SERVICE_DIRECTORIES" >&2
    exit 1
  fi

  local service_dir service snapshot_id rc
  local restored=() failed=()

  # Each service restores with auto-restore's DEFAULT flags (no -overwrite,
  # -delete, -hash, -ignore-owner unless the caller exported them): duplicacy
  # MERGES the snapshot into the destination — writes missing files, skips
  # existing ones, removes nothing. Non-destructive against whatever already
  # occupies the service dir, so a blank volume, a partially-restored re-run, or
  # a dir holding unrelated (e.g. read-only bind-mounted) files all restore cleanly.
  for service_dir in "${EXPANDED_SERVICE_DIRECTORIES[@]}"; do
    service="$(basename "${service_dir}")"
    # Snapshot id scheme mirrors set_duplicacy_variables() in duplicacy-backup.sh.
    snapshot_id="${HOSTNAME}-${service}"

    log_message ""
    log_message "=== ${service}: restoring '${snapshot_id}' -> ${service_dir} ==="
    SNAPSHOT_ID="${snapshot_id}" LOCAL_DIR="${service_dir}" "${AUTO_RESTORE_SCRIPT}"
    rc=$?
    if [ "${rc}" -eq 0 ]; then
      restored+=("${service}")
    else
      log_message "WARNING: restore failed for '${service}' (auto-restore exit ${rc})"
      failed+=("${service}")
    fi
  done

  log_message ""
  log_message "=== auto-restore-all summary ==="
  log_message "restored: ${restored[*]:-none}"
  if [ "${#failed[@]}" -gt 0 ]; then
    log_message "FAILED:   ${failed[*]}"
    exit 1
  fi
  log_message "All services restored."
}

main "$@"
