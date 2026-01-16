#!/bin/bash

source "/opt/archiver/lib/core/common.sh"

echo "This script will create a bundle file containing your config.sh and keys directory."
echo "The bundle will be encrypted and protected by a password you provide below."
echo "You must remember this password and keep a copy of the bundle file."

# Check if BUNDLE_PASSWORD environment variable is set and offer to reuse it
if [ -n "${BUNDLE_PASSWORD}" ]; then
    echo ""
    read -p "Reuse existing BUNDLE_PASSWORD environment variable? (Y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Nn]$ ]]; then
        PASSWORD="${BUNDLE_PASSWORD}"
        echo "Using existing password."
    fi
fi

# If PASSWORD not set (no env var or user declined), prompt for new password
if [ -z "${PASSWORD}" ]; then
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
fi

# Setup bundle directory
mkdir -p "${BUNDLE_DIR}"
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
chmod -R 600 "${BUNDLE_DIR}"/*

echo "Config and keys have been bundled as '${OUTPUT_ENC}'."
echo "Please keep a copy of this bundle file in a safe location."
echo

# Display SSH public key if it exists (useful for adding to SFTP servers)
if [ -f "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" ]; then
  echo "=========================================="
  echo "SSH Public Key (for SFTP server access):"
  echo "=========================================="
  cat "${DUPLICACY_SSH_PUBLIC_KEY_FILE}"
  echo
  echo "Copy this key to your SFTP server's authorized_keys file if using SFTP storage."
fi
