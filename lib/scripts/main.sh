#!/bin/bash
#
# Archiver Main Backup Script
#

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"
source_if_not_sourced "${DUPLICACY_FEATURE}"

MAIN_SH_SOURCED=true

# Cleanup on exit
cleanup() {
  if [ "${early_exit}" != true ]; then
    log_lockfile_summary
    release_lock
    log_message "INFO" "Archiver main script exited."
  fi
}

trap cleanup EXIT

# Initialize script
initialize() {
  local lock_status

  # Store prune/retain argument
  ROTATION_OVERRIDE="${1}"

  # Acquire lock and handle result
  acquire_lock
  lock_status=$?

  if [ "${lock_status}" -eq 1 ]; then
    early_exit=true
    exit 1
  elif [ "${lock_status}" -eq 2 ]; then
    log_message "WARNING" "Stale lock file found. Cleaned up and proceeding."
  fi

  rotate_logs
  log_message "INFO" "${MAIN_SCRIPT} script started."
  duplicacy_binary_check
  verify_config
}

# Process a single service backup
process_service() {
  local service_dir="${1}"

  cd "${service_dir}" || { handle_error "Failed to change to '${service_dir}'. Continuing."; return 1; }

  SERVICE_DIR="${service_dir}"
  SERVICE="$(basename "${PWD}")"
  log_message "INFO" "Processing '${SERVICE}' service."

  set_duplicacy_variables

  # Set defaults before sourcing service-specific settings
  DUPLICACY_FILTERS_PATTERNS=("+*")
  service_specific_pre_backup_function() { :; }
  service_specific_post_backup_function() { :; }

  # Source service-specific settings if available
  if [ -f "${service_dir}/service-backup-settings.sh" ]; then
    source "${service_dir}/service-backup-settings.sh" || \
      log_message "WARNING" "Failed to import service-backup-settings.sh for '${SERVICE}'."
  fi

  log_message "INFO" "Starting backup for '${SERVICE}'."

  service_specific_pre_backup_function
  duplicacy_primary_backup || { handle_error "Backup failed for '${SERVICE}'."; return 1; }
  service_specific_post_backup_function
  duplicacy_add_backup || { handle_error "Add backup failed for '${SERVICE}'."; return 1; }

  unset SERVICE
  return 0
}

# Send completion notification
send_completion_notification() {
  local start_time
  local end_time
  local elapsed_time
  local total_time_taken

  start_time=$(get_backup_start_time)
  end_time=$(date +%s)
  elapsed_time=$((end_time - start_time))
  total_time_taken=$(format_duration "${elapsed_time}")

  local timestamp
  local message
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  message="[${timestamp}] [${HOSTNAME}] Archiver completed in ${total_time_taken} with ${ERROR_COUNT} error(s)."

  echo "${message}"
  notify "Archiver Script Completed" "${message}"
}

# Main backup orchestration
main() {
  local last_working_dir=""

  # Process each service directory
  for service_dir in "${EXPANDED_SERVICE_DIRECTORIES[@]}"; do
    if process_service "${service_dir}"; then
      last_working_dir="${service_dir}"
    fi
  done

  # Run prune from the final service directory
  # Per https://forum.duplicacy.com/t/prune-command-details/1005 only one repository should run prune
  cd "${last_working_dir}" || handle_error "Failed to change to '${last_working_dir}' for prune."

  duplicacy_wrap_up "${STORAGE_TARGET_1_NAME}" || handle_error "Wrap-up failed."
  duplicacy_copy_backup

  record_state_change "completed"
  send_completion_notification
}

# Entry point
initialize "${1}"
main
