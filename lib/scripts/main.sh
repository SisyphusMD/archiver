#!/bin/bash
# Main backup orchestration script

MAIN_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"
source_if_not_sourced "${DUPLICACY_BACKUP_FEATURE}"
source_if_not_sourced "${RECOVERY_KIT_FEATURE}"

cleanup() {
  if [ "${early_exit}" != true ]; then
    log_lockfile_summary
    release_lock
    log_message "INFO" "Main backup script exited."
  fi
}

trap cleanup EXIT

initialize() {
  local lock_status

  acquire_lock
  lock_status=$?

  if [ "${lock_status}" -eq 1 ]; then
    echo "A backup is already running (PID $(get_lock_pid)). Not starting another." >&2
    early_exit=true
    exit 1
  elif [ "${lock_status}" -eq 2 ]; then
    log_message "WARNING" "Stale lock file found. Cleaned up and proceeding."
  fi

  rotate_logs
  log_message "INFO" "Main backup script started."
  duplicacy_binary_check
  verify_config
}

process_service() {
  local service_dir="${1}"

  cd "${service_dir}" || { handle_error "Failed to change to ${service_dir}. Continuing."; return 1; }

  SERVICE_DIR="${service_dir}"
  SERVICE="$(basename "${PWD}")"
  log_message "INFO" "Processing ${SERVICE} service."

  set_duplicacy_variables

  # Set defaults before sourcing service-specific settings
  DUPLICACY_FILTERS_PATTERNS=("+*")
  service_specific_pre_backup_function() { :; }
  service_specific_post_backup_function() { :; }

  local settings="${service_dir}/service-backup-settings.sh" syntax_error
  if [ -f "${settings}" ]; then
    # A settings file that does not parse would leave the defaults in place: everything
    # backed up with no pre hook (so no database dump), reported as a success.
    if ! syntax_error="$(bash -n "${settings}" 2>&1)"; then
      handle_error "service-backup-settings.sh does not parse, so this service's backup is skipped: ${syntax_error}"
      return 1
    fi
    source "${settings}" || \
      log_message "WARNING" "Failed to import service-backup-settings.sh for ${SERVICE} service."
  fi

  log_message "INFO" "Starting backup for ${SERVICE} service."

  local pre_status=0 backup_status=0 post_status=0
  update_lock_stage "service:${service_dir}" "pre-backup"
  service_specific_pre_backup_function || pre_status=$?

  if [ "${pre_status}" -ne 0 ]; then
    # The service's files are in an unknown state (a half-written or stale dump). Backing
    # them up would make that the newest revision, the one auto-restore picks.
    handle_error "Pre-backup hook failed for ${SERVICE} service (exit ${pre_status}); skipping its backup so the newest revision stays the last good one."
    backup_status=1
  elif ! is_stop_requested; then
    update_lock_stage "service:${service_dir}" "backup"
    # duplicacy_primary_backup reports its own failures; a stop-triggered return is not an error.
    duplicacy_primary_backup || backup_status=1
  fi

  # The post hook undoes the pre hook (restarts what it stopped), so it runs whenever the
  # pre hook ran: after a failed pre hook, a failed backup, or a stop, not only a clean run.
  update_lock_stage "service:${service_dir}" "post-backup"
  service_specific_post_backup_function || post_status=$?
  if [ "${post_status}" -ne 0 ]; then
    handle_error "Post-backup hook failed for ${SERVICE} service (exit ${post_status}); check that whatever its pre hook stopped is running again."
  fi
  [ "${backup_status}" -eq 0 ] || return 1
  if ! is_stop_requested; then
    duplicacy_add_backup || { handle_error "Add backup failed for ${SERVICE} service."; return 1; }
  fi

  update_lock_stage "duplicacy" "backup"

  # If stop was requested, call stop script to handle kill + notifications. Target 'backup'
  # explicitly: the argless default is 'all', which would also stop a concurrently-running
  # maintenance run the user never asked to stop.
  if is_stop_requested; then
    log_message "INFO" "Stop requested. Service cleanup complete, invoking stop handler."
    "${STOP_SCRIPT}" backup
    # Should not reach here, but exit just in case
    exit 0
  fi

  unset SERVICE
  return 0
}

send_completion_notification() {
  local start_time
  local end_time
  local elapsed_time
  local total_time_taken

  start_time=$(get_backup_start_time)
  end_time=$(date +%s)
  elapsed_time=$((end_time - start_time))
  total_time_taken=$(format_duration "${elapsed_time}")

  local message

  if [ "${ERROR_COUNT}" -eq 0 ]; then
    message="Completed successfully in ${total_time_taken}."
  elif [ "${ERROR_COUNT}" -eq 1 ]; then
    message="Completed in ${total_time_taken} with 1 error."
  else
    message="Completed in ${total_time_taken} with ${ERROR_COUNT} errors."
  fi

  echo "${message}"
  notify "Backup Complete" "${message}"
}

main() {
  local last_working_dir=""

  for service_dir in "${EXPANDED_SERVICE_DIRECTORIES[@]}"; do
    # Once a stop is requested, starting another service would run its pre hook (stopping
    # its database, say) for a backup that will never happen.
    is_stop_requested && break
    if process_service "${service_dir}"; then
      last_working_dir="${service_dir}"
    fi
  done

  update_lock_stage "duplicacy" "post-backup"

  # A stop during the final service's backup returns from process_service before reaching
  # its stop handler; honor it here or the wrap-up below would run a destructive prune.
  if is_stop_requested; then
    log_message "INFO" "Stop requested. Skipping storage wrap-up, invoking stop handler."
    "${STOP_SCRIPT}" backup
    # Should not reach here, but exit just in case
    exit 0
  fi

  # cd "" is a silent no-op, so guard explicitly: with no successful service there is
  # nothing to copy, and duplicacy would run from an arbitrary repository directory.
  if [ -z "${last_working_dir}" ]; then
    handle_error "No service completed a backup. Skipping storage copies."
    record_state_change "completed"
    send_completion_notification
    return
  fi

  # Copies are repository-context commands; run them from the final service directory.
  cd "${last_working_dir}" || handle_error "Failed to change to ${last_working_dir} for copies."

  update_lock_stage "duplicacy" "copy"
  duplicacy_copy_backup

  # Recovery-kit failures are reported (handle_error -> notification) but never abort the
  # run: the state file leaves failed targets unrecorded so the next run retries them.
  run_recovery_kit

  record_state_change "completed"
  send_completion_notification
}

initialize
main

# Exit non-zero if any per-service errors accumulated. Detached mode
# (`archiver backup --detach`) backgrounds this script via `setsid nohup` so the exit
# code is discarded — logs and the optional Pushover notification are the signal there.
# Synchronous mode (`archiver backup`, or the entrypoint's `run backup`) propagates this
# exit through `archiver.sh` to the container, so K8s Jobs / CI pipelines see Failed on
# per-service errors.
if [ "${ERROR_COUNT:-0}" -gt 0 ]; then
  exit 1
fi
exit 0
