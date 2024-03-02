#!/bin/bash
#
# Archiver Main Script
# Performs backup operations on services located in a specified directory, creating archives,
# cleaning old backups, and managing Duplicacy backups.
#
# Usage instructions: Execute the script without arguments (./main.sh).

# Configuration Section
# ---------------------

# Define primary configuration variables for Archiver.
ARCHIVER_DIR="$(dirname "$(readlink -f "$0")")" # Path to Archiver directory
ARCHIVER_LOG_DIR="${ARCHIVER_DIR}/logs" # Path to Archiver logs directory

# Define primary Duplicacy-related configuration variables.
DUPLICACY_BIN="/usr/local/bin/duplicacy" # Path to Duplicacy binary
DUPLICACY_KEY_DIR="${ARCHIVER_DIR}/.keys" # Path to Duplicacy key directory
DUPLICACY_SSH_KEY_FILE="${DUPLICACY_KEY_DIR}/id_rsa" # SSH key file
DUPLICACY_RSA_PUBLIC_KEY_FILE="${DUPLICACY_KEY_DIR}/public.pem" # Path to RSA public key file for Duplicacy
DUPLICACY_RSA_PRIVATE_KEY_FILE="${DUPLICACY_KEY_DIR}/private.pem" # Path to RSA private key file for Duplicacy

# Set service agnostic variables
DATE="$(date +'%Y-%m-%d')"
DATETIME="$(date +'%Y-%m-%d_%H%M%S')" # Current date and time for backup naming
PARENT_DIR="/srv" # Parent directory where services reside
REQUIRED_VARS=( # List of required service-defined variables
  "ARCHIVE_FILES"
  "EXCLUDE_FILES"
)

# Define log file paths. Add new log files to the following array to ensure they're included in rotation.
ARCHIVER_LOG_FILE="${ARCHIVER_LOG_DIR}/archiver.log" # Log file for Archiver logs
DUPLICACY_LOG_FILE="${ARCHIVER_LOG_DIR}/duplicacy-output.log" # Log file for Duplicacy output
DOCKER_LOG_FILE="${ARCHIVER_LOG_DIR}/docker-output.log" # Log file for Docker output

# Array of log file variables, make sure to add more log file variables to this array if adding to the list above
ALL_LOG_FILES=(
  "ARCHIVER_LOG_FILE"
  "DUPLICACY_LOG_FILE"
  "DOCKER_LOG_FILE"
)

# Secrets
SECRETS_FILE="${ARCHIVER_DIR}/.keys/secrets.sh" && source "${SECRETS_FILE}" # Import secrets from secrets file

# OMV Duplicacy variables
DUPLICACY_OMV_STORAGE_NAME="omv" # Name of onsite storage for Duplicacy omv storage
DUPLICACY_OMV_STORAGE_URL="${OMV_URL}" # URL for onsite storage for Duplicacy omv storage
DUPLICACY_OMV_SSH_KEY_FILE="${DUPLICACY_SSH_KEY_FILE}" # SSH key file for Duplicacy omv storage
DUPLICACY_OMV_PASSWORD="${STORAGE_PASSWORD}" # Password for Duplicacy omv storage
DUPLICACY_OMV_RSA_PASSPHRASE="${RSA_PASSPHRASE}" # Passphrase for Duplicacy omv storage

# B2 Duplicacy varibles
DUPLICACY_BACKBLAZE_STORAGE_NAME="${backblaze}"
DUPLICACY_BACKBLAZE_STORAGE_URL="${B2_URL}"
DUPLICACY_BACKBLAZE_B2_ID="${B2_ID}"
DUPLICACY_BACKBLAZE_B2_KEY="${B2_KEY}"
DUPLICACY_BACKBLAZE_PASSWORD="${STORAGE_PASSWORD}"
DUPLICACY_BACKBLAZE_RSA_PASSPHRASE="${RSA_PASSPHRASE}" # Passphrase for Duplicacy omv storage

# Declare service specific variables (initially empty)
SERVICE="" # Name of the service
SERVICE_DIR="" # Full path to the service directory
BACKUP_DIR="" # Directory to store service backups
DUPLICACY_REPO_DIR="" # Directory for various Duplicacy repos
DUPLICACY_FILTERS_FILE="" # Location for Duplicacy filters file
DUPLICACY_SNAPSHOT_ID="" # Snapshot ID for Duplicacy
SOURCE_FILES=() # List of potential source file paths
DUPLICACY_FILTERS_PATTERNS=() # Include/exclude patterns for Duplicacy filter

# Function Definitions
# ---------------------

# Sets service-specific settings based on the service directory name provided.
# Parameters:
#   1. Service Directory Name: The name of the service directory to set settings for.
# Output:
#   None directly. Modifies global variables specific to the service for backup operations.
set_service_settings() {
  # Set service-specific variables

  SERVICE="${1}" # Name of the service
  SERVICE_DIR="${PARENT_DIR}/${SERVICE}" # Full path to the service directory
  BACKUP_DIR="${SERVICE_DIR}/backups" # Directory to store backups
  DUPLICACY_REPO_DIR="${BACKUP_DIR}/.duplicacy" # Directory for various Duplicacy repos
  DUPLICACY_FILTERS_FILE="${DUPLICACY_REPO_DIR}/filters" # Location for Duplicacy filters file
  DUPLICACY_SNAPSHOT_ID="${HOSTNAME}-${SERVICE}" # Snapshot ID for Duplicacy

  # Set potential source file paths
  SOURCE_FILES=(
    "${SERVICE_DIR}/service-backup-settings.sh"
  )

  # Set include/exclude patterns for Duplicacy filter
  DUPLICACY_FILTERS_PATTERNS=(
    "+*.${SERVICE}-backup.tar"
    "-*"
  )

  log_message "INFO" "Updated service-specific variables for ${SERVICE} service. Ready for backup operations."

  # Unset all vars from the REQUIRED_VARS array in case they had previously been set from another service
  unset_required_vars

  # Defining service_specific_pre_backup_function as an empty function in case it was previously defined from another service
  service_specific_pre_backup_function() { :; }
  # Defining service_specific_post_backup_function as an empty function in case it was previously defined from another service
  service_specific_post_backup_function() { :; }
  log_message "INFO" "Cleared pre/post backup functions for ${SERVICE} service to ensure fresh environment for next operation."
}

# Unsets the required variables for the backup process to clean up the environment.
# Parameters:
#   None. Operates on a predefined set of global variables that need to be unset.
# Output:
#   Clears the specified global variables from the environment. No direct output.
unset_required_vars() {
  # Iterate over each variable in the REQUIRED_VARS array
  for var in "${REQUIRED_VARS[@]}"; do
    # Unset the current variable
    unset "${var}" || handle_error "Unable to unset the variable '${var}' for the ${SERVICE} service. Ensure the variable name is correct and retry."
    # Log a message indicating that the variable has been unset
    log_message "INFO" "Variable '${var}' successfully unset for ${SERVICE} service, ensuring a clean state for the next iteration."
  done
}

# Logs a message to the archiver's log file with a timestamp.
# Parameters:
#   1. Log Level: The severity level of the log message (e.g., INFO, WARNING, ERROR).
#   2. Message: The log message to be recorded.
# Output:
#   Writes the log message to the archiver's log file. No console output except for WARNING or ERROR.
log_message() {
  local log_level
  local message
  local timestamp
  local target_log_file

  log_level="${1}"
  message="${2}"
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  target_log_file="${3:-${ARCHIVER_LOG_FILE}}" # Use ARCHIVER_LOG_FILE by default if no log file is specified

  echo "[${timestamp}] [${log_level}] [Service: ${SERVICE}] ${message}" >> "${target_log_file}" || \
    handle_error "Failed to log message for ${SERVICE} service to ${target_log_file}. Check if the log file is writable and disk space is available."

  # Print WARNING and ERROR messages to the terminal
  if [[ "${log_level}" == "WARNING" || "${log_level}" == "ERROR" ]]; then
    echo "[${timestamp}] [${log_level}] [Service: ${SERVICE}] ${message}"
  fi
}

# Logs output from backup operations, capturing both stdout and stderr streams.
# Parameters:
#   1. Output Message: The output message from the backup operation to be logged.
#   2. Log File (optional): Specifies the log file to write to. Defaults to the main archiver log file if not provided.
# Output:
#   Writes the provided output message to the specified log file or the default archiver log file. No console output.
log_output() {
  local target_log_file
  local log_level

  target_log_file="${1}"
  log_level="${2:-INFO}" # Use INFO log level by default if no log level is specified

  while IFS= read -r line; do
    log_message "${log_level}" "${line}" "${target_log_file}"
  done
}

# Rotates log files for the Archiver, ensuring log management adheres to a retention policy.
# Parameters:
#   1. Log File Path: The path to the log file to rotate.
# Output:
#   Old log files are archived or deleted according to the retention policy. No direct output.
rotate_logs() {
  # Enforces a retention policy by keeping only the specified maximum number of backup versions.
  # Parameters:
  #   1. Max Versions: The maximum number of backup versions to retain.
  #   2. Backup Directory: The directory containing the backup files to apply the retention policy to.
  # Output:
  #   Deletes older backup files exceeding the specified maximum count, preserving only the most recent versions.
  keep_max_versions() {
    local log_prefix
    local max_versions
    local log_files
    local num_files

    log_prefix="${1}"
    max_versions="${2}"

    # Find all log files matching the prefix and sort them in reverse order
    mapfile -t log_files < <(find "${ARCHIVER_LOG_DIR}" -name "${log_prefix}*.log" -type f -print0 | sort -rz)
    num_files="${#log_files[@]}"

    # Check if the number of log files exceeds the maximum allowed
    if [ "${num_files}" -gt "${max_versions}" ]; then
      # Iterate over log files starting from the (max_versions - 1) index
      # and remove any excess log files beyond the maximum allowed versions
      for (( i = max_versions - 1; i < num_files; i++ )); do
        rm -f "${log_files[i]}" || handle_error "Failed to remove file ${log_files[i]} for ${SERVICE} service. Verify file permissions and that the file is not in use."
        log_message "INFO" "Removed old log file: ${log_files[i]}"
      done
    fi
  }

  local log_file
  local log_name
  local log_type
  local max_versions

  log_file="${1}"
  log_name="$(basename "${log_file}")"
  log_type="${log_name%.*}"
  max_versions=7

  # Check if the log directory exists, and create it if it doesn't
  [ -d "${ARCHIVER_LOG_DIR}" ] || mkdir -p "${ARCHIVER_LOG_DIR}"

  # Rotate the log file if needed
  if [ -f "${log_file}" ]; then
    local creation_date

    # Extract the creation date of the log file
    creation_date="$(date -r "${log_file}" +'%Y-%m-%d')"

    # Check if the log file was created on a different date than today
    if [ "${creation_date}" != "${DATE}" ]; then
      local new_log_file

      # Generate a new log file name based on the log type and current date
      new_log_file="${ARCHIVER_LOG_DIR}/${log_type}-${creation_date}.log"

      # Rename the existing log file to the new name
      mv "${log_file}" "${new_log_file}" || \
        handle_error "Could not rename the log file from '${log_file}' to '${new_log_file}' for the ${SERVICE} service. Check file permissions and path validity."
      log_message "INFO" "Rotated log file: ${log_file} -> ${new_log_file}"

      # Keep a maximum of max_versions log files after rotation
      keep_max_versions "${log_type}" "${max_versions}"
    fi
  fi
}

# Error Handling
# ---------------------

# Handles error scenarios by logging an error message and optionally exiting the script with a failure status.
# Parameters:
#   1. Error Message: The error message to log.
#   2. Exit Code (optional): The exit code to terminate the script with. If not provided, the script does not exit.
# Output:
#   Logs the error message to the standard error stream and exits the script if an exit code is provided.
handle_error() {
  local message
  local code

  message="${1}" # Error message to display
  code="${2:-1}" # Exit status (default: 1)

  if [[ -z "${RECURSIVE_CALL}" ]]; then
    export RECURSIVE_CALL=true
    log_message "ERROR" "${message}"
    unset RECURSIVE_CALL
  else
    # Direct fallback logging to stderr
    echo "${message}" >&2
  fi

  return "${code}" # Return with specified code, exiting the loop for the current service.
}

# Imports and Checks
# ---------------------

# Imports source files necessary for the backup operation of a specific service.
# Parameters:
#   None. Utilizes global variables to determine which sources to import.
# Output:
#   Sources are imported into the script's environment. No direct output.
import_sources() {
  local found_sources

  found_sources=false

  # Iterate over each source file path
  for file in "${SOURCE_FILES[@]}"; do
    # Check if the source file exists
    if [ -f "${file}" ]; then
      found_sources=true
      # Source the file
      source "${file}" || handle_error "Sourcing the file '${file}' for the ${SERVICE} service failed. Verify the file exists and has the correct permissions."
      log_message "INFO" "Sourced configuration file '${file}' successfully for ${SERVICE} service."
    fi
  done

  # If no source files were found
  if [ "${found_sources}" = false ]; then
    log_message "WARNING" "Did not find any source files to import for ${SERVICE} service. Check configuration."
  fi
}

# Checks for the existence of required service-specific variables for the backup process.
# Parameters:
#   None. Uses global variables to determine which variables to check.
# Output:
#   Returns 0 if all required variables are set, non-zero otherwise. No direct output.
check_variables() {
  local missing_vars

  # Check if each required service-specific variable is set
  for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
      missing_vars+=("${var}")
    fi
  done

  # If there are any missing required service-specific variables, send an error message
  if [ "${#missing_vars[@]}" -gt 0 ]; then
    handle_error "Required variables not set for the ${SERVICE} service: ${missing_vars[*]}. Ensure all required variables are defined in the service's backup settings."
  else
    log_message "INFO" "All required service-specific variables are set for the ${SERVICE} service."
  fi
}

# Verifies if a specified directory exists and is accessible.
# Parameters:
#   1. Directory Path: The path to the directory to check.
# Output:
#   Returns 0 if the directory exists and is accessible, non-zero otherwise. No direct output.
check_directory() {
  local dir

  dir="${1}" # Directory to check
  [ -d "${dir}" ] || handle_error "The directory '${dir}' does not exist. Verify the path is correct and accessible."
}

# Archiving Functions
# ---------------------

# Creates a compressed archive of the service's data for backup.
# Parameters:
#   None directly. Uses global variables for determining source files and backup destination.
# Output:
#   Generates a .tar archive of the service's data in the specified backup directory. No direct output.
create_backup_archive() {
  # Build the exclude options
  local EXCLUDE_OPTIONS=()
  for exclude_file in "${EXCLUDE_FILES[@]}"; do
    EXCLUDE_OPTIONS+=("--exclude=${exclude_file}") # Construct exclude options
  done

  local backup_file
  local retry_attempts

  retry_attempts=3 # Number of retry attempts

  # Retry loop (because if tar gives the WARNING <file changed as we read it>, it throws an error code
  local attempt

  attempt=1
  while [ "${attempt}" -le "${retry_attempts}" ]; do
    # Run any specific pre backup commands defined in the source file
    service_specific_pre_backup_function

    log_message "INFO" "Creating backup archive for ${SERVICE} service. Attempt ${attempt}."

    backup_file="${BACKUP_DIR}/${DATETIME}.${SERVICE}-backup.tar"

    # Create the backup archive
    sudo tar -cf "${backup_file}" \
      "${EXCLUDE_OPTIONS[@]}" \
      -C "${PARENT_DIR}" \
      "${ARCHIVE_FILES[@]}"  2>&1 | log_output "${ARCHIVER_LOG_FILE}" "WARNING"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -eq 0 ]; then
      break # Break out of the loop if tar succeeds
    fi

    # If tar fails, delete the created backup file, and wait for a few seconds before retrying
    log_message "WARNING" "Failed to create backup archive for ${SERVICE} service on attempt ${attempt}."
    rm -f "${backup_file}" || handle_error "Failed to remove backup file ${backup_file} for ${SERVICE} service after unsuccessful backup attempt. Ensure the file is not locked and you have sufficient permissions."
    sleep 5
    ((attempt++))
  done

  # Check if the loop exited due to success or reaching the maximum attempts
  if [ "${attempt}" -gt "${retry_attempts}" ]; then
    handle_error "Could not create the backup archive for the ${SERVICE} service after ${retry_attempts} attempts. Check for write permissions and sufficient disk space."
  fi

  # Verify the backup archive exists and has a non-zero size
  [ -s "${backup_file}" ] || handle_error "The backup archive '${backup_file}' for the ${SERVICE} service is empty or missing. Ensure the backup process completes successfully."
  log_message "INFO" "Backup archive created successfully for ${SERVICE} service on attempt ${attempt}."

  # Run any specific post backup commands defined in the source file
  service_specific_post_backup_function
}

# Removes old backup archives, keeping only the most recent ones based on a retention policy.
# Parameters:
#   None. Utilizes global variables to find and manage backup archives.
# Output:
#   Older backups beyond the retention limit are deleted. No direct output.
clean_old_backups() {
  local num_backups

  log_message "INFO" "Removing old backups for ${SERVICE} service."
  find "${BACKUP_DIR}/" -name "*.${SERVICE}-backup.tar" -type f -printf '%T@ %p\n' | \
    sort -n | \
    head -n -7 | \
    cut -d ' ' -f 2- | \
    xargs rm -f || \
    handle_error "Failed to remove old ${SERVICE} backups."

  # Verify that old backups are removed
  num_backups=$(find "${BACKUP_DIR}/" -name "*.${SERVICE}-backup.tar" -type f | wc -l)
  [ "${num_backups}" -le 7 ] || handle_error "Unable to remove old backups for the ${SERVICE} service. Check for sufficient permissions and that the backup files are not in use."
  log_message "INFO" "Old backups removed successfully for ${SERVICE} service."
}

# Duplicacy Functions
# ---------------------

# Initializes Duplicacy for the service's backup directory if not already done.
# Parameters:
#   None. Operates within the context of the current service's backup directory.
# Output:
#   Initializes the Duplicacy repository in the backup directory. No direct output.
initialize_duplicacy() {
  local exit_status

  if [ ! -d "${DUPLICACY_REPO_DIR}" ]; then
    export DUPLICACY_OMV_SSH_KEY_FILE # Export SSH key file for omv storage so Duplicacy binary can see variable
    export DUPLICACY_OMV_PASSWORD # Export password for omv storage so Duplicacy binary can see variable
    export DUPLICACY_BACKBLAZE_B2_ID # Export BackBlaze Account ID for backblaze storage so Duplicacy binary can see variable
    export DUPLICACY_BACKBLAZE_B2_KEY # Export BackBlaze Application Key for backblaze storage so Duplicacy binary can see variable
    export DUPLICACY_BACKBLAZE_PASSWORD # Export password for backblaze storage so Duplicacy binary can see variable

    # Initialize OMV Duplicacy Storage
    log_message "INFO" "Initializing OMV Duplicacy Storage for ${SERVICE} service."
    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${DUPLICACY_OMV_STORAGE_NAME}" \
      "${DUPLICACY_SNAPSHOT_ID}" "${DUPLICACY_OMV_STORAGE_URL}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "OMV Duplicacy Storage initialization for the ${SERVICE} service failed. Ensure Duplicacy is installed and the repository path is correct."
    fi

    # Set Password for OMV Duplicacy Storage
    "${DUPLICACY_BIN}" set -key password -value "${DUPLICACY_OMV_PASSWORD}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the OMV Duplicacy Storage password for the ${SERVICE} service failed. Ensure the storage password is correctly specified in the service's backup settings or environment variables."
    fi

    # Set RSA Passphrase for OMV Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_OMV_STORAGE_NAME}" -key rsa_passphrase \
      -value "${DUPLICACY_OMV_RSA_PASSPHRASE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the OMV Duplicacy Storage RSA Passphrase for the ${SERVICE} service failed. Ensure the storage RSA Passphrase is correctly specified in the service's backup settings or environment variables."
    fi

    # Set SSH key file for OMV Duplicacy Storage
    "${DUPLICACY_BIN}" set -key ssh_key_file -value "${DUPLICACY_OMV_SSH_KEY_FILE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the OMV Duplicacy Storage SSH key file for the ${SERVICE} service failed. Verify the SSH key file path and permissions."
    fi

    # Verify OMV Duplicacy Storage initiation
    "${DUPLICACY_BIN}" check -storage "${DUPLICACY_OMV_STORAGE_NAME}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Verification of the OMV Duplicacy Storage initialization for the ${SERVICE} service failed. Ensure Duplicacy has been properly initialized."
    fi
    log_message "INFO" "OMV Duplicacy Storage initialization verified for ${SERVICE} service."

    # Add BackBlaze Duplicacy Storage
    log_message "INFO" "Adding BackBlaze Duplicacy Storage for ${SERVICE} service."
    "${DUPLICACY_BIN}" add -e -copy "${DUPLICACY_OMV_STORAGE_NAME}" -bit-identical -key \
      "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" \
      "${DUPLICACY_SNAPSHOT_ID}" "${DUPLICACY_BACKBLAZE_STORAGE_URL}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Adding BackBlaze Duplicacy Storage for the ${SERVICE} service failed. Ensure the BackBlaze variables are correctly specified in the secrets file."
    fi

    # Set Password for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key password \
      -value "${DUPLICACY_BACKBLAZE_PASSWORD}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage password for the ${SERVICE} service failed. Ensure the storage password is correctly specified in the service's backup settings or environment variables."
    fi

    # Set RSA Passphrase for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key rsa_passphrase \
      -value "${DUPLICACY_BACKBLAZE_RSA_PASSPHRASE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage RSA Passphrase for the ${SERVICE} service failed. Ensure the storage RSA Passphrase is correctly specified in the service's backup settings or environment variables."
    fi

    # Set Key ID for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key b2_id \
      -value "${DUPLICACY_BACKBLAZE_B2_ID}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Key ID for the ${SERVICE} service failed. Ensure the Key ID is correctly specified in the service's backup settings or environment variables."
    fi

    # Set Application Key for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key b2_key \
      -value "${DUPLICACY_BACKBLAZE_B2_KEY}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Application Key for the ${SERVICE} service failed. Ensure the Application Key is correctly specified in the service's backup settings or environment variables."
    fi

    # Verify BackBlaze Duplicacy Storage initialization
    "${DUPLICACY_BIN}" check -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Verification of the BackBlaze Duplicacy Storage initialization for the ${SERVICE} service failed. Ensure Duplicacy has been properly initialized."
    fi
    log_message "INFO" "BackBlaze Duplicacy Storage initialization verified for ${SERVICE} service."

    # Prepare Duplicacy filters file
    # Check if the filters file already exists
    if [ ! -e "${DUPLICACY_FILTERS_FILE}" ]; then
      touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the ${SERVICE} service. Check file permissions and ensure the directory structure is correct."
    else
      log_message "WARNING" "Duplicacy filters file already exists for ${SERVICE} service. Appending to existing file."
    fi

    # Add Duplicacy filters patterns to the filters file
    for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
      echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the ${SERVICE} service. Check file permissions and ensure the directory structure is correct."
    done

    # Log success message
    log_message "INFO" "Preparation for Duplicacy filter for ${SERVICE} service completed successfully."

    # Verify Duplicacy initialization
    [ -d "${DUPLICACY_REPO_DIR}" ] || handle_error "Unable to verify Duplicacy initialization for the ${SERVICE} service. The repository directory '${DUPLICACY_REPO_DIR}' does not exist. Ensure Duplicacy has been properly initialized and the repository directory is correctly specified."
    log_message "INFO" "Duplicacy initialized successfully for ${SERVICE} service."
  else
    log_message "INFO" "Duplicacy already initialized for ${SERVICE} service."
  fi
}

# Runs the Duplicacy backup operation for the current service's data.
# Parameters:
#   None. Uses configured Duplicacy settings and operates within the service's backup context.
# Output:
#   Performs a Duplicacy backup. Output is logged to the Duplicacy log file.
backup_duplicacy() {
  local exit_status

  # Run the Duplicacy backup to the OMV Storage
  log_message "INFO" "Running OMV Duplicacy Storage backup for ${SERVICE} service."
  "${DUPLICACY_BIN}" backup -storage "${DUPLICACY_OMV_STORAGE_NAME}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running the OMV Duplicacy Storage backup for the ${SERVICE} service failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "The OMV Duplicacy Storage backup completed successfully for ${SERVICE} service."

  # Verify OMV Duplicacy Storage backup completion
  "${DUPLICACY_BIN}" check -storage "${DUPLICACY_OMV_STORAGE_NAME}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Verification of the OMV Duplicacy Storage backup for the ${SERVICE} service failed. Check the backup integrity and storage accessibility."
  fi
  log_message "INFO" "OMV Duplicacy Storage backup verified for ${SERVICE} service."

  # Run the Duplicacy backup to the BackBlaze Storage
  log_message "INFO" "Running BackBlaze Duplicacy Storage backup for ${SERVICE} service."
  "${DUPLICACY_BIN}" copy -id "${DUPLICACY_SNAPSHOT_ID}" \
    -from "${DUPLICACY_OMV_STORAGE_NAME}" -to "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" \
    -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running the BackBlaze Duplicacy Storage backup for the ${SERVICE} service failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "The BackBlaze Duplicacy Storage backup completed successfully for ${SERVICE} service."

  # Verify BackBlaze Duplicacy Storage backup completion
  "${DUPLICACY_BIN}" check -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Verification of the BackBlaze Duplicacy Storage backup for the ${SERVICE} service failed. Check the backup integrity and storage accessibility."
  fi
  log_message "INFO" "BackBlaze Duplicacy Storage backup verified for ${SERVICE} service."
}

# Runs the Duplicacy prune operation for all repositories.
# Parameters:
#   None. Uses configured Duplicacy settings and operates within the final service's backup context.
# Output:
#   Performs a Duplicacy prune. Output is logged to the Duplicacy log file.
prune_duplicacy() {
  local exit_status

  # Prune the OMV Duplicacy Storage
  log_message "INFO" "Running OMV Duplicacy Storage prune for all repositories."
  "${DUPLICACY_BIN}" prune -all -storage "${DUPLICACY_OMV_STORAGE_NAME}" \
    -keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running OMV Duplicacy Storage prune failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "OMV Duplicacy Storage prune completed successfully."

  # Prune the BackBlaze Duplicacy Storage
  log_message "INFO" "Running BackBlaze Duplicacy Storage prune for all repositories."
  "${DUPLICACY_BIN}" prune -all -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" \
    -keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running BackBlaze Duplicacy Storage prune failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "BackBlaze Duplicacy Storage prune completed successfully."
}

# Main Function
# ---------------------

# Main function orchestrating the backup process across all services found in the parent directory.
# Parameters:
#   None. Operates based on configured global variables for directory paths and backup settings.
# Output:
#   Coordinates the backup process for each service, logging progress and results. No direct output.
main() {
  # For loop to iterate over directories within parent directory and perform main function on each
  for dir in "${PARENT_DIR}"/*/ ; do
    # Set service-specific variables and functions
    set_service_settings "$(basename "${dir}")" || { handle_error "Failed to set service settings for $(basename "${dir}"). Continuing to next service."; continue; }

    # Check if the service-backup-settings.sh file exists, and only execute the main function in that directory if it does
    if [ -f "${SERVICE_DIR}/service-backup-settings.sh" ]; then
      log_message "INFO" "Starting backup process for ${SERVICE} service." || { handle_error "Failed to log the start of the backup process for ${SERVICE}. Continuing to next operation."; continue; }

      # Import any available source files
      import_sources || { handle_error "Failed to import sources for ${SERVICE}. Verify source files and paths. Continuing to next operation."; continue; }

      # Check if required service-specific variables are sourced
      check_variables || { handle_error "Required variables not set for ${SERVICE}. Ensure all required variables are defined. Continuing to next operation."; continue; }

      # Move to service directory or exit if failed
      check_directory "${SERVICE_DIR}" || { handle_error "Service directory ${SERVICE_DIR} not found for ${SERVICE}. Verify the directory exists. Continuing to next operation."; continue; }
      cd "${SERVICE_DIR}" || { handle_error "Failed to change to service directory ${SERVICE_DIR} for ${SERVICE}. Continuing to next operation."; continue; }

      # Make sure backup directory exists
      check_directory "${BACKUP_DIR}" || { handle_error "Backup directory ${BACKUP_DIR} not found for ${SERVICE}. Verify the backup directory exists. Continuing to next operation."; continue; }

      # Create backup archive
      create_backup_archive || { handle_error "Failed to create backup archive for ${SERVICE}. Check permissions and disk space. Continuing to next operation."; continue; }

      # Remove old backups (keep latest 7)
      clean_old_backups || { handle_error "Failed to clean old backups for ${SERVICE}. Check permissions and existing backups. Continuing to next operation."; continue; }

      # Move to backup directory or exit if failed
      cd "${BACKUP_DIR}" || { handle_error "Failed to change to backup directory ${BACKUP_DIR} for backup operations of ${SERVICE}. Continuing to next operation."; continue; }

      # Initialize Duplicacy if not already initialized
      initialize_duplicacy || { handle_error "Duplicacy initialization failed for ${SERVICE}. Ensure Duplicacy is installed and configured correctly. Continuing to next operation."; continue; }

      # Run Duplicacy backup
      backup_duplicacy || { handle_error "Duplicacy backup failed for ${SERVICE}. Review Duplicacy logs for details. Continuing to next operation."; continue; }

      # Success message
      log_message "INFO" "Completed backup and duplication process successfully for ${SERVICE} service." || { handle_error "Failed to log the successful completion of backup and duplication for ${SERVICE}."; }
    else
      log_message "WARNING" "Skipped backup for ${SERVICE} service due to missing service-backup-settings.sh file. Check service configuration." || { handle_error "Failed to log warning for missing service-backup-settings.sh file for ${SERVICE}."; }
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
  prune_duplicacy || { handle_error "Duplicacy prune failed. Review Duplicacy logs for details."; }
}

# Execution Flow
# ---------------------

# Rotate the logs if needed
# Iterate over the array and call rotate_logs for each log file
for log_file in "${ALL_LOG_FILES[@]}"; do
    rotate_logs "${log_file}"
done

# Execute main function
main
