reset_service_settings() {
  # Reset (or declare) service-specific variables
  SERVICE_DIR="" # Full path to the service directory
  PARENT_DIR=""
  BACKUP_DIR="" # Directory to store service backups
  DUPLICACY_REPO_DIR="" # Directory for various Duplicacy repos
  DUPLICACY_FILTERS_FILE="" # Location for Duplicacy filters file
  DUPLICACY_SNAPSHOT_ID="" # Snapshot ID for Duplicacy

  log_message "INFO" "Successfully reset all service-specific variables."

  # Unset all vars from the REQUIRED_VARS array in case they had previously been set from another service
  # Iterate over each variable in the REQUIRED_VARS array
  for var in "${REQUIRED_VARS[@]}"; do
    # Unset the current variable
    unset "${var}" || handle_error "Unable to unset the variable '${var}'. Ensure the variable name is correct and retry."
  done

  # Log a message indicating that the variables have been unset
  log_message "INFO" "Required variables successfully unset, ensuring a clean state for the next iteration."

  # Defining service_specific_pre_backup_function as an empty function in case it was previously defined from another service
  service_specific_pre_backup_function() { :; }
  # Defining service_specific_post_backup_function as an empty function in case it was previously defined from another service
  service_specific_post_backup_function() { :; }

  log_message "INFO" "Cleared pre/post service-specific backup functions to ensure fresh environment for next operation."
}

# Sets service-specific settings based on the service directory name provided.
# Parameters:
#   1. Service Directory Path: The path of the service directory to set settings for.
# Output:
#   None directly. Modifies global variables specific to the service for backup operations.
set_service_settings() {
  # Set service-specific variables

  SERVICE_DIR="${1}" # Full path to the service directory
  PARENT_DIR="$(realpath "$(dirname "${SERVICE_DIR}")")"

  if [[ -n "${SEPARATE_BACKUP_DIR}" ]]; then
    BACKUP_DIR="${SERVICE_DIR}/${SEPARATE_BACKUP_DIR}"
  else
    BACKUP_DIR="${SERVICE_DIR}"
  fi
  
  DUPLICACY_REPO_DIR="${BACKUP_DIR}/.duplicacy" # Directory for various Duplicacy repos
  DUPLICACY_FILTERS_FILE="${DUPLICACY_REPO_DIR}/filters" # Location for Duplicacy filters file
  DUPLICACY_SNAPSHOT_ID="${HOSTNAME}-${SERVICE}" # Snapshot ID for Duplicacy

  log_message "INFO" "Updated service-specific variables for ${SERVICE} service. Ready for backup operations."
}