#!/bin/bash
# Interactive restore

RESTORE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${DUPLICACY_RESTORE_FEATURE}"

# Simple log_message for user output (restore is interactive)
log_message() {
  echo "$@"
}

count_storage_targets
verify_target_settings
check_required_secrets

SELECTED_STORAGE_TARGET_ID=""
SELECTED_STORAGE_TARGET_NAME=""
SELECTED_STORAGE_TARGET_TYPE=""
SNAPSHOT_ID=""
LOCAL_DIR=""
REVISION=""

select_storage_target() {
  local storage_targets=()

  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    local storage_id="${i}"
    local storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
    local storage_name="${!storage_name_var}"

    storage_targets+=("${storage_id}) ${storage_name}")
  done

  echo "Which of the following storage targets would you like to restore from?"
  for target in "${storage_targets[@]}"; do
    echo " - ${target}"
  done

  local choice
  while true; do
    read -rp "Enter the number of your choice: " choice
    if [[ "${choice}" =~ ^[0-9]+$ ]] && [ "${choice}" -ge 1 ] && [ "${choice}" -le "${STORAGE_TARGET_COUNT}" ]; then
      break
    else
      echo "Invalid choice. Please enter a valid number between 1 and ${STORAGE_TARGET_COUNT}."
    fi
  done

  resolve_storage_target_by_id "${choice}" || \
    handle_error "Failed to resolve storage target ${choice}."

  if [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "sftp" ]]; then
    if [ ! -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ] || [ ! -f "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" ]; then
      handle_error "Unable to find your SSH key files. This script requires your backed up SSH id_ed25519 and id_ed25519.pub files from the keys directory to restore from an SFTP storage target. Please restore those before running this script."
    fi
  fi
}

local_restore_selections() {
  echo    # Move to a new line
  while [ -z "${SNAPSHOT_ID}" ]; do
    echo    # Move to a new line
    read -rp "Snapshot ID to restore (required, list of available snapshot IDs can be found in the target storage 'snapshots' directory): " SNAPSHOT_ID
    if [ -z "${SNAPSHOT_ID}" ]; then
      echo "Error: Snapshot ID is required."
    fi
  done
  echo "Chosen Snapshot ID: ${SNAPSHOT_ID}"

  while [ -z "${LOCAL_DIR}" ]; do
    echo    # Move to a new line
    read -rp "Local directory path to restore to (required): " LOCAL_DIR
    if [ -z "${LOCAL_DIR}" ]; then
      echo "Error: Local directory path is required."
    fi
  done
  echo "Chosen local directory path: ${LOCAL_DIR}"
}

prepare_local_dir() {
  if [ ! -d "${LOCAL_DIR}" ]; then
    mkdir -p -m 0755 "${LOCAL_DIR}"
  fi

  cd "${LOCAL_DIR}" || handle_error "Failed to change directory to '${LOCAL_DIR}'."
}

choose_revision() {
  echo
  duplicacy list #this should give you the info for revision number, needed below
  echo

  while [ -z "${REVISION}" ]; do
    echo    # Move to a new line
    read -rp "Choose a revision number to restore (required): " REVISION
    if [ -z "${REVISION}" ]; then
      echo "Error: Revision number is required."
    fi
  done
  echo "Chosen revision: ${REVISION}"
}

configure_restore_options() {
  # Ask if user wants to customize restore options
  echo    # Move to a new line
  read -p "Customize restore options (advanced)? (y/N): " -n 1 -r
  echo    # Move to a new line

  RESTORE_FLAGS=""
  RESTORE_THREADS="${DUPLICACY_THREADS}"

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Ask about hash-based detection
    read -p "Detect file differences by hash (slower but more thorough)? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -hash"
    fi

    # Ask about overwriting files
    read -p "Overwrite existing files in the restore directory? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -overwrite"
    fi

    # Ask about deleting extra files
    read -p "Delete files not in the snapshot? (WARNING: Removes extra files) (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -delete"
    fi

    # Ask about ignoring ownership
    read -p "Ignore original file ownership (useful when restoring to different machine)? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -ignore-owner"
    fi

    # Ask about persistence on errors
    read -p "Continue even if errors occur? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -persist"
    fi

    # Ask about thread count
    read -p "Override download thread count (current: ${DUPLICACY_THREADS})? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      read -rp "Enter thread count: " RESTORE_THREADS
      if [ -z "${RESTORE_THREADS}" ]; then
        RESTORE_THREADS="${DUPLICACY_THREADS}"
      fi
    fi
  fi

  if [ -n "${RESTORE_FLAGS}" ]; then
    echo "Additional restore flags:${RESTORE_FLAGS}"
  fi
  if [ "${RESTORE_THREADS}" != "${DUPLICACY_THREADS}" ]; then
    echo "Download threads: ${RESTORE_THREADS}"
  fi
}

service_specific_restore_script() {
  if [ -f restore-service.sh ]; then
    echo    # Move to a new line
    read -p "Found a restore-service.sh file. Would you like to run it now? (y|N):" -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo "Now running restore-service.sh..."
      bash restore-service.sh
    else
      echo "Did not run restore-service.sh script."
    fi
  fi
}

main() {
  select_storage_target
  local_restore_selections
  prepare_local_dir
  duplicacy_init_for_restore
  choose_revision
  configure_restore_options
  perform_restore
  echo "Repository restored."
  service_specific_restore_script
}

main
