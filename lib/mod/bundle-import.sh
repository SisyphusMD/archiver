#!/bin/bash

# Archiver directory
ARCHIVER_DIR="/opt/archiver"
BUNDLE_DIR="${ARCHIVER_DIR}/bundle"

# Check if running in non-interactive mode (Docker)
if [ "${ARCHIVER_NON_INTERACTIVE}" = "1" ]; then
  # Non-interactive mode: use environment variables
  if [ -z "${ARCHIVER_BUNDLE_PASSWORD}" ]; then
    echo "Error: ARCHIVER_BUNDLE_PASSWORD not set in non-interactive mode"
    exit 1
  fi

  if [ -n "${ARCHIVER_BUNDLE_FILE}" ] && [ -f "${ARCHIVER_BUNDLE_FILE}" ]; then
    SELECTED_FILE="${ARCHIVER_BUNDLE_FILE}"
  else
    echo "Error: ARCHIVER_BUNDLE_FILE not set or file not found in non-interactive mode"
    exit 1
  fi

  PASSWORD="${ARCHIVER_BUNDLE_PASSWORD}"
else
  # Interactive mode: original behavior
  # Look for bundle.tar.enc in the archiver directory or bundle directory
  if [ -f "${ARCHIVER_DIR}/bundle.tar.enc" ]; then
    SELECTED_FILE="${ARCHIVER_DIR}/bundle.tar.enc"
  elif [ -f "${BUNDLE_DIR}/bundle.tar.enc" ]; then
    SELECTED_FILE="${BUNDLE_DIR}/bundle.tar.enc"
  else
    echo "No bundle file found."
    echo "Expected: ${ARCHIVER_DIR}/bundle.tar.enc or ${BUNDLE_DIR}/bundle.tar.enc"
    exit 1
  fi

  echo "Found bundle file: ${SELECTED_FILE}"

  # Prompt for password to decrypt the file
  echo "Enter password to decrypt the bundle file:"
  read -rs PASSWORD
fi

# Define temporary output tar file
TEMP_TAR="${SELECTED_FILE%.enc}"

# Decrypt the selected bundle file
openssl enc -d -aes-256-cbc -pbkdf2 -in "${SELECTED_FILE}" -out "${TEMP_TAR}" -k "${PASSWORD}"
if [ $? -ne 0 ]; then
  echo "Error: Decryption failed. Please check your password and try again."
  rm -f "${TEMP_TAR}"
  exit 1
fi

# Extract the decrypted tarball to a temporary directory
TEMP_DIR="${ARCHIVER_DIR}/temp_import"
mkdir -p "${TEMP_DIR}"
tar -xf "${TEMP_TAR}" -C "${TEMP_DIR}"
if [ $? -ne 0 ]; then
  echo "Error: Extraction failed. Please check the tarball and try again."
  rm -f "${TEMP_TAR}"
  rm -rf "${TEMP_DIR}"
  exit 1
fi

# Remove the decrypted tarball
rm -f "${TEMP_TAR}"

# Backup existing config.sh and keys directory if they exist
if [ -f "${ARCHIVER_DIR}/config.sh" ]; then
  mv "${ARCHIVER_DIR}/config.sh" "${ARCHIVER_DIR}/config.sh.bckp"
fi

if [ -d "${ARCHIVER_DIR}/keys" ]; then
  mv "${ARCHIVER_DIR}/keys" "${ARCHIVER_DIR}/keys.bckp"
fi

# Move the extracted files to their original locations
mv "${TEMP_DIR}/config.sh" "${ARCHIVER_DIR}/config.sh"
mkdir -p "${ARCHIVER_DIR}/keys"
mv "${TEMP_DIR}/keys"/* "${ARCHIVER_DIR}/keys/"
# Set permissions
chmod 700 "${ARCHIVER_DIR}/keys"
chmod 600 "${ARCHIVER_DIR}/keys/private.pem"
chmod 644 "${ARCHIVER_DIR}/keys/public.pem"
chmod 600 "${ARCHIVER_DIR}/keys/id_ed25519"
chmod 644 "${ARCHIVER_DIR}/keys/id_ed25519.pub"
chmod 600 "${ARCHIVER_DIR}/config.sh"

# Clean up temporary directory
rm -rf "${TEMP_DIR}"

# In non-interactive mode (Docker), skip moving the bundle file since it's mounted read-only
if [ "${ARCHIVER_NON_INTERACTIVE}" != "1" ]; then
  # Move bundle file to bundle directory
  # Setup bundle directory
  mkdir -p "${BUNDLE_DIR}"
  chmod -R 700 "${BUNDLE_DIR}"
  # Set permissions of imported file and move to bundle directory
  chmod 600 "${SELECTED_FILE}"
  mv "${SELECTED_FILE}" "${BUNDLE_DIR}"
fi

echo "Import completed successfully. Existing config.sh and keys have been backed up with .bckp suffix."
