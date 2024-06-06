#!/bin/bash

# Determine archiver repo directory path by traversing up the directory tree until we find 'archiver.sh' or reach the root
EXPORT_SCRIPT_PATH="$(realpath "$0")"
CURRENT_DIR="$(dirname "${EXPORT_SCRIPT_PATH}")"
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

# Define exports directories
EXPORTS_DIR="${ARCHIVER_DIR}/exports"

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

echo "This export script will create an archive file containing your config.sh file and keys directory for backup."
echo "The file will be encrypted, and protected by a password you provide below."
echo "You must remember this password, and keep a copy of the export file created by this script."

# Prompt for password twice and compare
while true; do
    echo "Enter password to encrypt the export:"
    read -rs PASSWORD
    echo
    echo "Re-enter password to confirm:"
    read -rs PASSWORD_CONFIRM
    echo
    if [ "${PASSWORD}" == "${PASSWORD_CONFIRM}" ]; then
        break
    else
        echo "Passwords do not match. Please try again."
    fi
done

# Get the current timestamp
TIMESTAMP=$(date +"%Y%m%d-%H%M%S")

# Get the UID and GID of the user who invoked the script
CALLER_UID=$(id -u "${SUDO_USER}")
CALLER_GID=$(id -g "${SUDO_USER}")

# Setup exports directory
mkdir -p "${EXPORTS_DIR}"
chown -R "${CALLER_UID}:${CALLER_GID}" "${EXPORTS_DIR}"
chmod 700 "${EXPORTS_DIR}"

# Define the output tar file name
OUTPUT_TAR="${EXPORTS_DIR}/export-${TIMESTAMP}.tar"

# Define the files and directories to be tarred
INPUT_FILES=("${ARCHIVER_DIR}/keys/" "${ARCHIVER_DIR}/config.sh")

# Create tarball
tar -cvf "${OUTPUT_TAR}" "${INPUT_FILES[@]}"

# Encrypt tarball
openssl enc -aes-256-cbc -salt -in "${OUTPUT_TAR}" -out "${OUTPUT_TAR}.enc" -k "${PASSWORD}"

# Remove unencrypted tarball
rm "${OUTPUT_TAR}"

# Set file permissions
chown -R "${CALLER_UID}:${CALLER_GID}" "${EXPORTS_DIR}"
chmod -R 600 "${EXPORTS_DIR}"/*

echo "Config and key files have been exported as '${OUTPUT_TAR}.enc'."
echo "Please keep a separate backup of this file."
