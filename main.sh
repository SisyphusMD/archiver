#!/bin/bash
#
# Archiver Main Script
# Performs Duplicacy backup operations on services located in a specified directory
#
# Usage instructions: Execute the script as sudo without arguments (sudo ./main.sh).

# Configuration Section
# ---------------------

# Capture the start time
START_TIME=$(date +%s)

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user." 1>&2
 exit 1
fi

# Define primary configuration variables for Archiver.
# Try to resolve the full path to this Archiver script with readlink
ARCHIVER_SCRIPT_PATH=$(readlink -f "${0}" 2>/dev/null)
# If readlink is not successful (e.g., command not found, or operating in an environment like macOS without readlink), use BASH_SOURCE
if [[ ! -e "${ARCHIVER_SCRIPT_PATH}" ]]; then
    ARCHIVER_SCRIPT_PATH="${BASH_SOURCE[0]}"
fi
# Resolve to an absolute path if not already
ARCHIVER_DIR="$(cd "$(dirname "${ARCHIVER_SCRIPT_PATH}")" && pwd)"
# Check if we've got a valid directory, just in case
if [[ ! -d "${ARCHIVER_DIR}" ]]; then
    echo "Error determining the script's directory."
    exit 1
fi

ARCHIVER_LOG_DIR="${ARCHIVER_DIR}/logs" # Path to Archiver logs directory
# Define log file paths. Add new log files to the following array to ensure they're included in rotation.
ARCHIVER_LOG_FILE="${ARCHIVER_LOG_DIR}/archiver.log" # Log file for Archiver logs
DUPLICACY_LOG_FILE="${ARCHIVER_LOG_DIR}/duplicacy-output.log" # Log file for Duplicacy output
DOCKER_LOG_FILE="${ARCHIVER_LOG_DIR}/docker-output.log" # Log file for Docker output
CURL_LOG_FILE="${ARCHIVER_LOG_DIR}/curl-output.log" # Log file for Curl output
# Array of log file variables, make sure to add more log file variables to this array if adding to the list above
ALL_LOG_FILES=(
  "${ARCHIVER_LOG_FILE}"
  "${DUPLICACY_LOG_FILE}"
  "${DOCKER_LOG_FILE}"
  "${CURL_LOG_FILE}"
)

# Set service agnostic variables
DATE="$(date +'%Y-%m-%d')"
DATETIME="$(date +'%Y-%m-%d_%H%M%S')" # Current date and time for backup naming
REQUIRED_VARS=( # List of required service-defined variables
  "SERVICE"
  "DUPLICACY_FILTERS_PATTERNS"
)

# Sourcing configurable variables from config.sh.
# This file contains customizable variables that allow users to tailor the script's behavior to their specific needs and environment.
# Variables in config.sh may include paths, threshold settings, preferences, and other parameters that can be adjusted by the user.
# It's designed to make the script flexible and adaptable, without requiring modifications to the core script code.
# Please review and adjust the variables in config.sh as necessary to fit your setup.
# Configuration for Duplicacy is sourced from config.sh.
# This includes setting paths and keys essential for Duplicacy operations:
# - DUPLICACY_BIN: Path to the Duplicacy binary.
# - DUPLICACY_KEY_DIR: Directory containing Duplicacy keys.
# - DUPLICACY_SSH_KEY_FILE: SSH key file for Duplicacy.
# - DUPLICACY_RSA_PUBLIC_KEY_FILE: RSA public key file used by Duplicacy.
# - DUPLICACY_RSA_PRIVATE_KEY_FILE: RSA private key file used by Duplicacy.
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

# Script variables
ERROR_COUNT=0
LAST_BACKUP_DIR=""

# Service Functions
# ---------------------
source "${ARCHIVER_DIR}/utils/service-functions.sh"
# imports functions:
#   - reset_service_settings
#   - set_service_settings

# Logging
# ---------------------
source "${ARCHIVER_DIR}/utils/logging.sh"
# imports functions:
#   - log_message
#   - log_output
#   - rotate_logs
#   - elapsed_time

# Error Handling
# ---------------------
source "${ARCHIVER_DIR}/utils/error-handling.sh"
# imports functions:
#   - handle_error

# Checks
# ---------------------
source "${ARCHIVER_DIR}/utils/checks.sh"
# imports functions:
#   - check_variables
#   - check_directory

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

  # Initialize all service-specific variables and functions
  reset_service_settings

  # Loop to iterate over user-defined service directories and perform main function on each
  for dir in "${EXPANDED_DIRECTORIES[@]}" ; do
    # Move to user defined service directory or exit if failed
    check_directory "${dir}" || { handle_error "Service directory ${dir} not found. Verify the directory exists. Continuing to next operation."; continue; }
    cd "${dir}" || { handle_error "Failed to change to service directory ${dir}. Continuing to next operation."; continue; }
    log_message "INFO" "Successfully changed to the ${dir} directory." || { handle_error "Failed to log the successful change to the ${dir} directory. Continuing to next operation."; continue; }

    # Reset all service-specific variables and functions
    reset_service_settings

    # Check if the service-backup-settings.sh file exists, and only execute the main function in that directory if it does
    if [ -f "${dir}/service-backup-settings.sh" ]; then
      # Source the service-backup-settings.sh file
      source "${dir}/service-backup-settings.sh" || { handle_error "Failed to import ${dir}/service-backup-settings.sh. Verify source files and paths. Continuing to next operation."; continue; }

      log_message "INFO" "Starting backup process for ${dir} directory." || { handle_error "Failed to log the start of the backup process for the ${dir} directory. Continuing to next operation."; continue; }

      # Check if required service-specific variables are successfully sourced
      check_variables || { handle_error "Required variables not set for the ${dir} directory. Ensure all required variables are defined. Continuing to next operation."; continue; }

      # Set service-specific variables and functions
      set_service_settings "${dir}" || { handle_error "Failed to set service settings for the ${dir} directory. Continuing to next service."; continue; }

      # Make sure backup directory exists
      check_directory "${BACKUP_DIR}" || { handle_error "Backup directory ${BACKUP_DIR} not found for ${SERVICE}. Verify the backup directory exists. Continuing to next operation."; continue; }

      # Run any specific pre backup commands defined in the service-backup-settings.sh file
      service_specific_pre_backup_function

      # Run Duplicacy backup
      backup_duplicacy || { handle_error "Duplicacy backup failed for ${SERVICE}. Review Duplicacy logs for details. Continuing to next operation."; continue; }

      # Run any specific post backup commands defined in the source file
      service_specific_post_backup_function

      # Run Duplicacy copy backup
      copy_backup_duplicacy || { handle_error "Duplicacy copy backup failed for ${SERVICE}. Review Duplicacy logs for details. Continuing to next operation."; continue; }

      # Set last successful backup dir
      LAST_BACKUP_DIR="${BACKUP_DIR}"

      # Success message
      log_message "INFO" "Completed backup and duplication process successfully for ${SERVICE} service." || { handle_error "Failed to log the successful completion of backup and duplication for ${SERVICE}."; }

      # Reset all service-specific variables and functions
      reset_service_settings
    else
      log_message "WARNING" "Skipped backup for the ${dir} directory due to missing service-backup-settings.sh file. Check service configuration." || { handle_error "Failed to log warning for missing service-backup-settings.sh file for the ${dir} directory."; }
    fi
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
  # Move to last successful backup directory
  check_directory "${LAST_BACKUP_DIR}" || handle_error "Directory ${LAST_BACKUP_DIR} not found. Verify the directory exists."
  cd "${LAST_BACKUP_DIR}" || handle_error "Failed to change to directory ${LAST_BACKUP_DIR}. Continuing to next operation."

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
