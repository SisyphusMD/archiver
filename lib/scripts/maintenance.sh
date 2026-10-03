#!/bin/bash
# Maintenance pipeline orchestrator: per-storage check + prune, scheduled independently of
# backups (MAINTENANCE_SCHEDULE) so storage upkeep can never extend or block a backup run.
# `archiver maintenance [exhaustive]` — 'exhaustive' forces the full-listing prune now.

MAINTENANCE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi

# This pipeline owns its own lock, stop flag, and log file — point the shared helpers at
# them BEFORE sourcing the lockfile/logging chain.
ARCHIVER_LOCKFILE="${MAINTENANCE_LOCKFILE}"
ARCHIVER_STOP_FLAG_FILE="${MAINTENANCE_STOP_FLAG}"
ARCHIVER_LOG_BASENAME="maintenance"

source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"
source_if_not_sourced "${DUPLICACY_MAINTENANCE_FEATURE}"

FORCE_EXHAUSTIVE=false
[ "${1:-}" = "exhaustive" ] && FORCE_EXHAUSTIVE=true

cleanup() {
  if [ "${early_exit}" != true ]; then
    log_lockfile_summary "Maintenance"
    release_lock
    log_message "INFO" "Maintenance script exited."
  fi
}

trap cleanup EXIT

send_maintenance_notification() {
  local start_time end_time elapsed_time total_time_taken message
  start_time=$(get_backup_start_time)
  end_time=$(date +%s)
  elapsed_time=$((end_time - start_time))
  total_time_taken=$(format_duration "${elapsed_time}")

  if [ "${ERROR_COUNT}" -eq 0 ]; then
    message="Completed successfully in ${total_time_taken}."
  elif [ "${ERROR_COUNT}" -eq 1 ]; then
    message="Completed in ${total_time_taken} with 1 error."
  else
    message="Completed in ${total_time_taken} with ${ERROR_COUNT} errors."
  fi

  echo "${message}"
  notify "Maintenance Complete" "${message}"
}

initialize() {
  local lock_status

  acquire_lock
  lock_status=$?

  if [ "${lock_status}" -eq 1 ]; then
    echo "A maintenance run is already in progress (PID $(get_lock_pid)). Not starting another." >&2
    early_exit=true
    exit 1
  elif [ "${lock_status}" -eq 2 ]; then
    log_message "WARNING" "Stale maintenance lock file found. Cleaned up and proceeding."
  fi

  # acquire_lock seeds the lock line with the backup pipeline's "duplicacy pre-backup"
  # placeholder; relabel it so status/stop show a maintenance run correctly until the first
  # per-storage stage is set.
  update_lock_stage "maintenance" "starting"
  rotate_logs
  log_message "INFO" "Maintenance script started (exhaustive forced: ${FORCE_EXHAUSTIVE})."
  verify_config

  # verify_config normalizes PRUNE_BACKUPS (lowercases, applies the ROTATE_BACKUPS alias), so
  # test it only now: a forced exhaustive is inert when pruning is off and the run would
  # otherwise exit successfully without saying the force did nothing.
  if [ "${FORCE_EXHAUSTIVE}" = "true" ] && [ "${PRUNE_BACKUPS}" != "true" ]; then
    log_message "WARNING" "'exhaustive' requested but PRUNE_BACKUPS is false; no prune (exhaustive or otherwise) will run."
  fi

  # check/prune are repository-context commands; any INITIALIZED service repository knows
  # every storage (the backup pipeline configures them all). Scan for the first one that has
  # been initialized: a newly-added service that sorts first has no .duplicacy until its own
  # first backup, and must not block maintenance for every storage the others already know.
  local d repo_dir=""
  for d in "${EXPANDED_SERVICE_DIRECTORIES[@]}"; do
    [ -f "${d}/.duplicacy/preferences" ] && { repo_dir="${d}"; break; }
  done
  if [ -z "${repo_dir}" ]; then
    handle_error "No initialized repository found (expected a service dir with .duplicacy/preferences). Run a backup before maintenance."
    record_state_change "failed"
    exit 1
  fi
  cd "${repo_dir}" || { handle_error "Failed to change to ${repo_dir}."; record_state_change "failed"; exit 1; }
}

# Ask the daemon about the workers for THIS run's storages: ctl fingerprints them from its
# environment, where bash's loaded settings are not exported. Only the non-secret storage
# fields are passed, so no secret reaches another process's environment.
daemon_ctl_for_storages() {
  (
    while IFS= read -r v; do
      [[ "${v}" == STORAGE_TARGET_* ]] && export "${v?}"
    done < <(compgen -v | grep -E "${CONFIG_NONSECRET_VARS_RE}")
    /usr/local/bin/archiver daemon ctl "$1" 2>/dev/null
  )
}

end_stopped() {
  log_message "INFO" "Stop requested; ending maintenance early."
  record_state_change "stopped"
  notify "Maintenance Stopped" "Stopped before completing all storages."
  MAINTENANCE_STOPPED=true
}

main() {
  local i rc

  if [[ "${CHECK_BACKUPS}" != "true" && "${PRUNE_BACKUPS}" != "true" ]]; then
    log_message "INFO" "CHECK_BACKUPS and PRUNE_BACKUPS are both false; nothing to do."
    record_state_change "completed"
    return
  fi

  # Copy workers maintain the secondaries themselves (check, mirror local's retention,
  # exhaustive prune: ADRs 12, 17, 18); pruning them here with -keep as well would bring
  # back the race mirroring removes.
  local last_storage="${STORAGE_TARGET_COUNT}"
  if [ "$(daemon_ctl_for_storages workers)" = "ok" ]; then
    last_storage=1
    [ "${STORAGE_TARGET_COUNT}" -gt 1 ] && log_message "INFO" "Secondary storages are maintained by their copy workers; maintaining local only."
    if [ "${FORCE_EXHAUSTIVE}" = "true" ] && [ "${PRUNE_BACKUPS}" = "true" ] && [ "$(daemon_ctl_for_storages exhaustive)" = "ok" ]; then
      log_message "INFO" "Copy workers will prune their secondary storages exhaustively on their next pass."
    fi
  fi

  for i in $(seq 1 "${last_storage}"); do
    if is_stop_requested; then
      end_stopped; return
    fi
    maintain_storage "${i}"
    rc=$?
    if [ "${rc}" -eq 130 ]; then
      end_stopped; return
    fi
  done

  # A stop during the last listing must not be followed by the wake below.
  if is_stop_requested; then
    end_stopped; return
  fi
  # A local prune is a change the workers mirror onto the secondaries; a run that pruned
  # nothing is not (and must not end a stop, which only a change to local does).
  if [ "${LOCAL_PRUNED:-false}" = true ]; then
    daemon_ctl_for_storages local-changed >/dev/null || true
  fi

  record_state_change "completed"
  send_maintenance_notification
}

initialize
main

# Synchronous verb: the scheduler and external schedulers see real failures, and a
# stopped pass is not a completed one.
if [ "${ERROR_COUNT:-0}" -gt 0 ] || [ "${MAINTENANCE_STOPPED:-false}" = true ]; then
  exit 1
fi
exit 0
