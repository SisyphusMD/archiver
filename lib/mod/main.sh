#!/bin/bash
#
# Archiver Main Script
# Performs Duplicacy backup operations on services located in specified directories
#
# Usage instructions: This script is intended to be run by archiver.sh, rather than invoking directly.

# ---------------------
# Initial Setup
# ---------------------
# Time Variables
START_TIME="$(date +%s)"
DATE="$(date +'%Y-%m-%d')"
DATETIME="$(date +'%Y-%m-%d_%H%M%S')"

# Define unique identifier for the script (e.g., script's full path)
MAIN_SCRIPT_PATH="$(realpath "$0")"
LOCKFILE="/var/lock/archiver-$(echo "${MAIN_SCRIPT_PATH}" | md5sum | cut -d' ' -f1).lock"

# Determine archiver repo directory path by traversing up the directory tree until we find 'archiver.sh' or reach the root
CURRENT_DIR="$(dirname "${MAIN_SCRIPT_PATH}")"
ARCHIVER_DIR=""
while [ "${CURRENT_DIR}" != "/" ]; do
  if [ -f "${CURRENT_DIR}/archiver.sh" ]; then
    ARCHIVER_DIR="${CURRENT_DIR}"
    break
  fi
  CURRENT_DIR="$(dirname "${CURRENT_DIR}")"
done

# Check if we found the file
if [ -z "${ARCHIVER_DIR}" ]; then
  echo "Error: archiver.sh not found in any parent directory."
  exit 1
fi

# Define lib, src, mod, log, logo directories
LOG_DIR="${ARCHIVER_DIR}/logs"
OLD_LOG_DIR="${LOG_DIR}/prior_logs"
LIB_DIR="${ARCHIVER_DIR}/lib"
SRC_DIR="${LIB_DIR}/src"
MOD_DIR="${LIB_DIR}/mod"
LOGO_DIR="${LIB_DIR}/logos"

# Define module scripts
LOGS_SCRIPT="${MOD_DIR}/logs.sh"
STOP_SCRIPT="${MOD_DIR}/archiver.sh"

# Source all the necessary src files
for script in "${SRC_DIR}/"*.sh; do
  [ -r "${script}" ] && source "${script}"
  # ---------------------
  # Configuration Check
  # ---------------------
  # set-config.sh
  #
  # imports functions:
  #   - verify_config
  #   - expand_service_directories
  #   - count_storage_targets
  #   - verify_target_settings
  #   - check_required_secrets
  #   - check_notification_config
  #   - check_backup_rotation_settings

  # ---------------------
  # Logging
  # ---------------------
  # logging.sh
  #
  # imports variables:
  #   - LOG_DIR
  #   - ARCHIVER_LOG_FILE
  #   - DUPLICACY_LOG_FILE
  #   - DOCKER_LOG_FILE
  #   - CURL_LOG_FILE
  #   - ALL_LOG_FILES
  # imports functions:
  #   - log_message
  #   - log_output
  #   - rotate_logs
  #   - elapsed_time

  # ---------------------
  # Error Handling
  # ---------------------
  # error-handling.sh
  # imports variables:
  #   - ERROR_COUNT
  # imports functions:
  #   - handle_error

  # ---------------------
  # Duplicacy Functions
  # ---------------------
  # duplicacy.sh
  # imports functions:
  #   - set_duplicacy_variables
  #   - duplicacy_binary_check
  #   - count_storage_targets
  #   - duplicacy_verify
  #   - duplicacy_filters
  #   - duplicacy_primary_backup
  #   - duplicacy_copy_backup
  #   - duplicacy_prune

  # ---------------------
  # Notification Functions
  # ---------------------
  # notification.sh
  # imports functions:
  #   - send_pushover_notification
  #   - notify
done

# Function to remove the lock file upon script exit
cleanup() {
  if [ "${early_exit}" != true ]; then
    rm -f "${LOCKFILE}"
    log_message "INFO" "Archiver main script exited."
  fi
}

# Trap signals to ensure cleanup is performed
trap cleanup EXIT

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

# Store argument in case of prune or retain
ROTATION_OVERRIDE="${1}"

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_INFO="$(cat "${LOCKFILE}")"
  LOCK_PID="$(echo "${LOCK_INFO}" | cut -d' ' -f1)"
  LOCK_SCRIPT="$(echo "${LOCK_INFO}" | cut -d' ' -f2)"

  if [ -n "${LOCK_PID}" ] && [ "${LOCK_SCRIPT}" = "${MAIN_SCRIPT_PATH}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    # Set early_exit to true to avoid removing the lockfile
    early_exit=true
    exit 1
  else
    log_message "WARNING" "Stale lock file found. Cleaning up."
    rm -f "${LOCKFILE}"
  fi
fi

# Check for running instances using pgrep excluding the current process
pgrep_output=$(pgrep -f "${MAIN_SCRIPT_PATH}" | grep -v "^$$\$")
if [ -n "${pgrep_output}" ]; then
  echo "An instance of ${MAIN_SCRIPT_PATH} is still running, even with no LOCKFILE present. Stopping the process."

  # Kill the running instance(s) and their child processes
  while read -r pid; do
    # Terminate the process and its children
    pkill -TERM -P "${pid}"
    if kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}"
      echo "Killed running instance of ${MAIN_SCRIPT_PATH} with PID: ${pid}"
    fi
  done <<< "${pgrep_output}"
fi

# Create the lock file with the current PID and script path
echo "$$ ${MAIN_SCRIPT_PATH}" > "${LOCKFILE}"

# Rotate the logs
rotate_logs

log_message "INFO" "${MAIN_SCRIPT_PATH} script started."

# Make sure duplicacy binary is installed
duplicacy_binary_check

# Verify configuration settings and export defaults and expanded directories array
verify_config

# ---------------------
# Main Function
# ---------------------

# Main function orchestrating the backup process across all services found in the parent directory.
# Parameters:
#   None. Operates based on configured global variables for directory paths and backup settings.
# Output:
#   Coordinates the backup process for each service, logging progress and results. No direct output.
main() {
  # Loop to iterate over user-defined service directories and perform backup function on each
  for SERVICE_DIR in "${EXPANDED_SERVICE_DIRECTORIES[@]}" ; do
    # Move to user defined service directory or exit if failed
    cd "${SERVICE_DIR}" || { handle_error "Failed to change to the '${SERVICE_DIR}' service directory. Continuing to the next service directory."; continue; }
    LAST_WORKING_DIR="${SERVICE_DIR}"

    # Define service name from directory
    SERVICE="$(basename "${PWD}")"

    log_message "INFO" "Successfully changed to the '${SERVICE_DIR}' service directory."

    set_duplicacy_variables

    # Define default service variables before attempting to source file
    DUPLICACY_FILTERS_PATTERNS=("+*")
    service_specific_pre_backup_function() { :; }
    service_specific_post_backup_function() { :; }

    # Check if the service-backup-settings.sh file exists
    if [ -f "${SERVICE_DIR}/service-backup-settings.sh" ]; then
      log_message "INFO" "Found service-backup-settings.sh file for the '${SERVICE}' service. Attempting to import..."
      if source "${SERVICE_DIR}/service-backup-settings.sh"; then
        log_message "INFO" "Successfully imported service-backup-settings.sh file for the '${SERVICE}' service."
      else
        log_message "WARNING" "Failed to import service-backup-settings.sh file for the '${SERVICE}' service. Using default values."
      fi
    else
      log_message "INFO" "No service-backup-settings.sh file for the '${SERVICE}' service. Using default values."
    fi

    log_message "INFO" "Starting backup process for the '${SERVICE}' service." 

    # Run any specific pre backup commands defined in the service-backup-settings.sh file
    service_specific_pre_backup_function

    # Run Duplicacy primary backup
    duplicacy_primary_backup || { handle_error "Duplicacy backup failed for the '${SERVICE}' service. Review Duplicacy logs for details. Continuing to next operation."; continue; }

    # Run any specific post backup commands defined in the source file
    service_specific_post_backup_function

    # Run Duplicacy add backup
    duplicacy_add_backup || { handle_error "Duplicacy add backup failed for the '${SERVICE}' service. Review Duplicacy logs for details. Continuing to next operation."; continue; }

    # Unset SERVICE variable
    unset SERVICE
  done

  # Run Duplicacy prune function from the final service backup folder
  #
  # Per this page: https://forum.duplicacy.com/t/prune-command-details/1005
  #
  #   Only one repository should run prune
  #
  #   Since Duplicacy encourages multiple repositories backing up to the same storage (so that deduplication will be efficient),
  #     users might want to run prune from each different repository.
  #
  #   The design of Duplicacy however was based on the assumption that only one instance would run the prune command (using -all).
  #     This can greatly simplify the implementation.
  #
  # Move to last service directory
  cd "${LAST_WORKING_DIR}" || handle_error "Failed to change to the last working service directory, '${LAST_WORKING_DIR}', to complete the duplicacy prune."

  # Full check and prune the primary storage
  duplicacy_wrap_up "${STORAGE_TARGET_1_NAME}" || handle_error "Duplicacy wrap-up failed. Review Duplicacy logs for details."

  # For each storage, copy the backup, then full check and prune it
  duplicacy_copy_backup

  # Capture the end time
  END_TIME=$(date +%s)

  # Calculate the total runtime
  ELAPSED_TIME="$(("${END_TIME}" - "${START_TIME}"))"

  # Get the total runtime in human-readable format
  TOTAL_TIME_TAKEN="$(elapsed_time "${ELAPSED_TIME}")"

  # Send terminal and notification of script completion with error count
  local message
  local timestamp
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  message="[${timestamp}] [${HOSTNAME}] Archiver script completed in ${TOTAL_TIME_TAKEN} with ${ERROR_COUNT} error(s)."
  echo "${message}"
  notify "Archiver Script Completed" "${message}"
}

# Execution Flow
# ---------------------

# Execute main function
main
