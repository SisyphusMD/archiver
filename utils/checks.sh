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
    handle_error "Required variables not set for the ${dir} directory: ${missing_vars[*]}. Ensure all required variables are defined in the service's backup settings."
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
