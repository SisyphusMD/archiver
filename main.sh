#!/bin/bash
#
# Archiver Main Script
# Performs Duplicacy backup operations on services located in specified directories
#
# Usage instructions: Execute the script as sudo without arguments (sudo ./main.sh).

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user." 1>&2
 exit 1
fi

# Configuration Section
# ---------------------
# Set time variables
START_TIME="$(date +%s)"
DATE="$(date +'%Y-%m-%d')"
DATETIME="$(date +'%Y-%m-%d_%H%M%S')"

# Determine the full path of the script
ARCHIVER_SCRIPT_PATH="$(readlink -f "${0}" 2>/dev/null)"
# Determine the full path of the containing dir of the script
ARCHIVER_DIR="$(cd "$(dirname "${ARCHIVER_SCRIPT_PATH}")" && pwd)"

# Sourcing configurable variables from config.sh.
# Please review and adjust the variables in config.sh as necessary to fit your setup.
source "${ARCHIVER_DIR}/config.sh"
# Initialize an empty array to hold the directory paths
EXPANDED_DIRECTORIES=()
# Populate user defined backup directories into the EXPANDED_DIRECTORIES array
for pattern in "${BACKUP_REPOSITORIES[@]}"; do
  # Directly list directories for specific paths or wildcard patterns
  for dir in ${pattern}; do  # Important: Don't quote ${pattern} to allow glob expansion
    if [ -d "${dir}" ]; then
      # Add the directory to the array, removing the trailing slash
      EXPANDED_DIRECTORIES+=("${dir%/}")
    fi
  done
done

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
#   - verify_duplicacy
#   - initialize_duplicacy
#   - backup_duplicacy
#   - prune_duplicacy

# Notification Functions
# ---------------------
source "${ARCHIVER_DIR}/utils/notification.sh"
# imports functions:
#   - send_pushover_notification

# Main Function
# ---------------------

# Main function orchestrating the backup process across all services found in the parent directory.
# Parameters:
#   None. Operates based on configured global variables for directory paths and backup settings.
# Output:
#   Coordinates the backup process for each service, logging progress and results. No direct output.
main() {
  # Rotate the logs if needed
  # Iterate over the array and call rotate_logs for each log file
  for log_file in "${ALL_LOG_FILES[@]}"; do
      rotate_logs "${log_file}"
  done

  # Loop to iterate over user-defined service directories and perform main function on each
  for SERVICE_DIR in "${EXPANDED_DIRECTORIES[@]}" ; do
    # Move to user defined service directory or exit if failed
    cd "${SERVICE_DIR}" || { handle_error "Failed to change to service directory ${SERVICE_DIR}. Continuing to next operation."; continue; }

    # Define service name from directory
    SERVICE="$(basename "${PWD}")"

    log_message "INFO" "Successfully changed to the ${SERVICE} service directory."
    echo "${SERVICE_DIR}"
    echo "${DUPLICACY_FILTERS_FILE}"
    set_duplicacy_variables
    echo "${SERVICE_DIR}"
    echo "${DUPLICACY_FILTERS_FILE}"
    cat "${DUPLICACY_FILTERS_FILE}"

    # Define default service variables before attempting to source file
    DUPLICACY_FILTERS_PATTERNS=("+*")
    service_specific_pre_backup_function() { :; }
    service_specific_post_backup_function() { :; }

    echo "${DUPLICACY_FILTERS_PATTERNS[@]}"
    # Check if the service-backup-settings.sh file exists
    if [ -f "${SERVICE_DIR}/service-backup-settings.sh" ]; then
      # Attempt to source the file
      log_message "INFO" "Found service-backup-settings.sh file for ${SERVICE} service. Attempting to source..."
      source "${SERVICE_DIR}/service-backup-settings.sh" || \
        log_message "WARNING" "Failed to import service-backup-settings.sh file for ${SERVICE} service. Using default values."
    else
      # Log an informational message if the file does not exist
      log_message "INFO" "No service-backup-settings.sh file for ${SERVICE} service. Using default values."
    fi
    echo "${DUPLICACY_FILTERS_PATTERNS[@]}"

    log_message "INFO" "Starting backup process for ${SERVICE} service." 

    # Run any specific pre backup commands defined in the service-backup-settings.sh file
    service_specific_pre_backup_function

    # Run Duplicacy backup
    backup_duplicacy || { handle_error "Duplicacy backup failed for ${SERVICE}. Review Duplicacy logs for details. Continuing to next operation."; continue; }

    # Run any specific post backup commands defined in the source file
    service_specific_post_backup_function

    # Run Duplicacy copy backup
    copy_backup_duplicacy || { handle_error "Duplicacy copy backup failed for ${SERVICE}. Review Duplicacy logs for details. Continuing to next operation."; continue; }

    # Success message
    log_message "INFO" "Completed backup and duplication process successfully for ${SERVICE} service." || { handle_error "Failed to log the successful completion of backup and duplication for ${SERVICE}."; }

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
  cd "${SERVICE_DIR}" || handle_error "Failed to change to directory ${SERVICE_DIR}. Continuing to next operation."

  # Prune duplicacy from last successful backup directory
  prune_duplicacy || { handle_error "Duplicacy prune failed. Review Duplicacy logs for details."; }

  # Capture the end time
  END_TIME=$(date +%s)

  # Calculate the total runtime
  ELAPSED_TIME=$(($END_TIME - $START_TIME))

  # Get the total runtime in human-readable format
  TOTAL_TIME_TAKEN=$(elapsed_time $ELAPSED_TIME)

  # Send terminal and Pushover notification of script completion with error count
  local message
  local timestamp
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  message="[${timestamp}] [${HOSTNAME}] Archiver script completed in ${TOTAL_TIME_TAKEN} with ${ERROR_COUNT} error(s)."
  echo "${message}"
  send_pushover_notification "Archiver Script Completed" "${message}"
}

# Execution Flow
# ---------------------

# Execute main function
main
