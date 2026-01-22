#!/bin/bash
# Main backup orchestration script

MAIN_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"
source_if_not_sourced "${DUPLICACY_FEATURE}"

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

  ROTATION_OVERRIDE="${1}"

  acquire_lock
  lock_status=$?

  if [ "${lock_status}" -eq 1 ]; then
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

  if [ -f "${service_dir}/service-backup-settings.sh" ]; then
    source "${service_dir}/service-backup-settings.sh" || \
      log_message "WARNING" "Failed to import service-backup-settings.sh for ${SERVICE} service."
  fi

  log_message "INFO" "Starting backup for ${SERVICE} service."

  update_lock_stage "service:${service_dir}" "pre-backup"
  service_specific_pre_backup_function

  if ! is_stop_requested; then
    update_lock_stage "service:${service_dir}" "backup"
    duplicacy_primary_backup || { handle_error "Backup failed for ${SERVICE} service."; return 1; }
  fi

  # Always run post-backup hook after pre-backup
  update_lock_stage "service:${service_dir}" "post-backup"
  service_specific_post_backup_function
  if ! is_stop_requested; then
    duplicacy_add_backup || { handle_error "Add backup failed for ${SERVICE} service."; return 1; }
  fi

  update_lock_stage "duplicacy" "backup"

  # If stop was requested, call stop script to handle kill + notifications
  if is_stop_requested; then
    log_message "INFO" "Stop requested. Service cleanup complete, invoking stop handler."
    "${STOP_SCRIPT}"
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
    if process_service "${service_dir}"; then
      last_working_dir="${service_dir}"
    fi
  done

  update_lock_stage "duplicacy" "post-backup"

  # Run prune from the final service directory
  # Per https://forum.duplicacy.com/t/prune-command-details/1005 only one repository should run prune
  cd "${last_working_dir}" || handle_error "Failed to change to ${last_working_dir} for prune."

  local primary_storage_name
  primary_storage_name="$(sanitize_storage_name "${STORAGE_TARGET_1_NAME}")"

  duplicacy_wrap_up "${primary_storage_name}" || handle_error "Wrap-up failed."
  duplicacy_copy_backup

  record_state_change "completed"
  send_completion_notification
}

initialize "${1}"
main
