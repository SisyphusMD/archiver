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
    log_message "INFO" "Creating backup archive for ${SERVICE} service. Attempt ${attempt}."

    backup_file="${BACKUP_DIR}/${DATETIME}.${SERVICE}-backup.tar"

    # Create the backup archive
    sudo "${ARCHIVER_TAR}" "${backup_file}" \
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