#!/bin/bash
#
# Archiver Main Script
# Performs Duplicacy backup operations on services located in specified directories
#
# Usage instructions: Execute the script as sudo without arguments (sudo ./main.sh).

# ---------------------
# Initial Checks
# ---------------------

LOCKFILE="/var/lock/archiver.lock"
LOGFILE="/var/log/archiver.log"

# Function to log messages
log_message() {
  local message="$1"
  echo "$(date +"%Y-%m-%d %H:%M:%S") - ${message}" >> "${LOGFILE}"
}

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 log_message "This script must be run as root. Please use sudo or log in as the root user."
 exit 1
fi

# Function to remove the lock file upon script exit
cleanup() {
  rm -f "${LOCKFILE}"
  log_message "Archiver script ended."
}

# Trap signals to ensure cleanup is performed
trap cleanup EXIT

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_PID=$(cat "${LOCKFILE}")
  if [ -n "${LOCK_PID}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    log_message "Another instance of Archiver is already running with PID ${LOCK_PID}."
    exit 1
  else
    log_message "Stale lock file found. Cleaning up."
    rm -f "${LOCKFILE}"
  fi
fi

# Create the lock file with the current PID
echo $$ > "${LOCKFILE}"
log_message "Archiver script started."


# ---------------------
# Environment Variables
# ---------------------
START_TIME="$(date +%s)"
DATE="$(date +'%Y-%m-%d')"
DATETIME="$(date +'%Y-%m-%d_%H%M%S')"
MAIN_SCRIPT="$(readlink -f "${0}" 2>/dev/null)"
ARCHIVER_DIR="$(cd "$(dirname "${MAIN_SCRIPT}")" && pwd)"


# ---------------------
# Configuration Check
# ---------------------
source "${ARCHIVER_DIR}/utils/set-config.sh"
# imports functions:
#   - verify_config
#   - expand_service_directories
#   - count_storage_targets
#   - verify_target_settings
#   - check_required_secrets
#   - check_notification_config
#   - check_backup_rotation_settings

# Logging
# ---------------------
source "${ARCHIVER_DIR}/utils/logging.sh"
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

# Error Handling
# ---------------------
source "${ARCHIVER_DIR}/utils/error-handling.sh"
# imports variables:
#   - ERROR_COUNT
# imports functions:
#   - handle_error

# Duplicacy Functions
# ---------------------
source "${ARCHIVER_DIR}/utils/duplicacy.sh"
# imports functions:
#   - set_duplicacy_variables
#   - duplicacy_binary_check
#   - count_storage_targets
#   - duplicacy_verify
#   - duplicacy_filters
#   - duplicacy_primary_backup
#   - duplicacy_copy_backup
#   - duplicacy_prune

# Notification Functions
# ---------------------
source "${ARCHIVER_DIR}/utils/notification.sh"
# imports functions:
#   - send_pushover_notification
#   - notify

# Main Function
# ---------------------

# Main function orchestrating the backup process across all services found in the parent directory.
# Parameters:
#   None. Operates based on configured global variables for directory paths and backup settings.
# Output:
#   Coordinates the backup process for each service, logging progress and results. No direct output.
main() {
  # Rotate the logs
  rotate_logs

  # Make sure duplicacy binary is installed
  duplicacy_binary_check

  # Verify configuration settings and export defaults and expanded directories array
  verify_config

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

    # Run Duplicacy copy backup
    duplicacy_copy_backup || { handle_error "Duplicacy copy backup failed for the '${SERVICE}' service. Review Duplicacy logs for details. Continuing to next operation."; continue; }

    # Success message
    log_message "INFO" "Completed backup and duplication process successfully for the '${SERVICE}' service."

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

  # Prune duplicacy from last successful backup directory
  duplicacy_wrap_up || handle_error "Duplicacy wrap-up failed. Review Duplicacy logs for details."

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
