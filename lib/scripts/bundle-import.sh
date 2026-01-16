#!/bin/bash

BUNDLE_IMPORT_SH_SOURCED=true

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi

# Check if existing config or keys will be overwritten and warn user
HAS_CONFIG=false
HAS_KEYS=false
[ -f "${CONFIG_FILE}" ] && HAS_CONFIG=true
[ -d "${KEYS_DIR}" ] && [ -n "$(ls -A "${KEYS_DIR}" 2>/dev/null)" ] && HAS_KEYS=true

if [ "${HAS_CONFIG}" = true ] || [ "${HAS_KEYS}" = true ]; then
  echo ""
  echo "WARNING: Existing configuration and/or keys will be OVERWRITTEN:"
  [ "${HAS_CONFIG}" = true ] && echo "  - Configuration: ${CONFIG_FILE}"
  [ "${HAS_KEYS}" = true ] && echo "  - Keys: ${KEYS_DIR}/"
  echo ""
  echo "This operation is IRREVERSIBLE. Your existing configuration and keys will be lost."
  echo "If you need to preserve them, run 'archiver bundle export' first to create a backup."
  echo ""
  read -p "Do you want to continue? (y/N): " -r
  echo ""

  # Require explicit Y/y, default to No
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Import cancelled."
    exit 0
  fi
  echo ""
fi

# Verify required environment variables
if [ -z "${ARCHIVER_BUNDLE_PASSWORD}" ]; then
  echo "Error: ARCHIVER_BUNDLE_PASSWORD environment variable is required"
  exit 1
fi

if [ -z "${ARCHIVER_BUNDLE_FILE}" ] || [ ! -f "${ARCHIVER_BUNDLE_FILE}" ]; then
  echo "Error: ARCHIVER_BUNDLE_FILE not set or file not found"
  exit 1
fi

SELECTED_FILE="${ARCHIVER_BUNDLE_FILE}"
PASSWORD="${ARCHIVER_BUNDLE_PASSWORD}"

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

# Move the extracted files to their original locations
mv "${TEMP_DIR}/config.sh" "${CONFIG_FILE}"
mkdir -p "${KEYS_DIR}"
mv "${TEMP_DIR}/keys"/* "${KEYS_DIR}/"
# Set permissions
chmod 700 "${KEYS_DIR}"
chmod 600 "${DUPLICACY_RSA_PRIVATE_KEY_FILE}"
chmod 644 "${DUPLICACY_RSA_PUBLIC_KEY_FILE}"
chmod 600 "${DUPLICACY_SSH_PRIVATE_KEY_FILE}"
chmod 644 "${DUPLICACY_SSH_PUBLIC_KEY_FILE}"
chmod 600 "${CONFIG_FILE}"

# Clean up temporary directory
rm -rf "${TEMP_DIR}"

echo "Import completed successfully."
