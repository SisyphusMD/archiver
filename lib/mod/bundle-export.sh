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

# Define bundle directory
BUNDLE_DIR="${ARCHIVER_DIR}/bundle"

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

echo "This script will create a bundle file containing your config.sh and keys directory."
echo "The bundle will be encrypted and protected by a password you provide below."
echo "You must remember this password and keep a copy of the bundle file."

# Prompt for password twice and compare
while true; do
    echo "Enter password to encrypt the bundle:"
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

# Get the UID and GID of the user who invoked the script
if [ -n "${SUDO_USER}" ]; then
  CALLER_UID=$(id -u "${SUDO_USER}")
  CALLER_GID=$(id -g "${SUDO_USER}")
else
  # Running as root directly (e.g., in Docker)
  CALLER_UID=$(id -u)
  CALLER_GID=$(id -g)
fi

# Setup bundle directory
mkdir -p "${BUNDLE_DIR}"
chown -R "${CALLER_UID}:${CALLER_GID}" "${BUNDLE_DIR}"
chmod 700 "${BUNDLE_DIR}"

# Define the output file names (always bundle.tar / bundle.tar.enc)
OUTPUT_TAR="${BUNDLE_DIR}/bundle.tar"
OUTPUT_ENC="${OUTPUT_TAR}.enc"

# If bundle.tar.enc already exists, move it to bundle.tar.enc.old
if [ -f "${OUTPUT_ENC}" ]; then
  # Remove any existing .old file first
  [ -f "${OUTPUT_ENC}.old" ] && rm -f "${OUTPUT_ENC}.old"

  # Attempt to move the existing bundle to .old
  mv "${OUTPUT_ENC}" "${OUTPUT_ENC}.old"

  # Verify the backup was successful before continuing
  if [ -f "${OUTPUT_ENC}.old" ] && [ ! -f "${OUTPUT_ENC}" ]; then
    echo "Existing bundle backed up as bundle.tar.enc.old"
  else
    echo "Error: Failed to backup existing bundle file"
    echo "The file may be locked or in use. Please ensure no other processes are accessing it."
    exit 1
  fi
fi

# Change to the ARCHIVER_DIR directory and create the tarball
cd "${ARCHIVER_DIR}"
tar -cf "${OUTPUT_TAR}" keys config.sh

# Encrypt tarball with pbkdf2
openssl enc -aes-256-cbc -pbkdf2 -salt -in "${OUTPUT_TAR}" -out "${OUTPUT_ENC}" -k "${PASSWORD}"

# Remove unencrypted tarball
rm "${OUTPUT_TAR}"

# Set file permissions
chown -R "${CALLER_UID}:${CALLER_GID}" "${BUNDLE_DIR}"
chmod -R 600 "${BUNDLE_DIR}"/*

echo "Config and keys have been bundled as '${OUTPUT_ENC}'."
echo "Please keep a copy of this bundle file in a safe location."
echo

# Display SSH public key if it exists (useful for adding to SFTP servers)
SSH_PUB_KEY="${ARCHIVER_DIR}/keys/id_ed25519.pub"
if [ -f "${SSH_PUB_KEY}" ]; then
  echo "=========================================="
  echo "SSH Public Key (for SFTP server access):"
  echo "=========================================="
  cat "${SSH_PUB_KEY}"
  echo
  echo "Copy this key to your SFTP server's authorized_keys file if using SFTP storage."
fi
