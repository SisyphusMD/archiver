#!/bin/bash

BUNDLE_IMPORT_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi

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

# Outside the entrypoint (a 'docker exec ... archiver bundle import'), self-resolve the same
# way the entrypoint does: default bundle path, password from its secret file.
if [ -z "${ARCHIVER_BUNDLE_FILE}" ]; then
  ARCHIVER_BUNDLE_FILE="${BUNDLE_DIR}/bundle.tar.enc"
fi
if [ -z "${ARCHIVER_BUNDLE_PASSWORD}" ]; then
  bundle_password_path="${BUNDLE_PASSWORD_FILE:-${SECRETS_DIR}/bundle_password}"
  if [ -f "${bundle_password_path}" ]; then
    ARCHIVER_BUNDLE_PASSWORD="$(<"${bundle_password_path}")"
    ARCHIVER_BUNDLE_PASSWORD="${ARCHIVER_BUNDLE_PASSWORD%$'\r'}"
  fi
fi

if [ -z "${ARCHIVER_BUNDLE_PASSWORD}" ]; then
  echo "Error: no bundle password found at ${BUNDLE_PASSWORD_FILE:-${SECRETS_DIR}/bundle_password}"
  exit 1
fi

if [ ! -f "${ARCHIVER_BUNDLE_FILE}" ]; then
  echo "Error: bundle file not found at ${ARCHIVER_BUNDLE_FILE}"
  exit 1
fi

SELECTED_FILE="${ARCHIVER_BUNDLE_FILE}"
PASSWORD="${ARCHIVER_BUNDLE_PASSWORD}"

TEMP_TAR="${SELECTED_FILE%.enc}"

# -pass fd: keeps the password off the openssl argv (world-readable in /proc while it runs).
openssl enc -d -aes-256-cbc -pbkdf2 -in "${SELECTED_FILE}" -out "${TEMP_TAR}" -pass fd:3 3<<<"${PASSWORD}"
if [ $? -ne 0 ]; then
  echo "Error: Decryption failed. Please check your password and try again."
  rm -f "${TEMP_TAR}"
  exit 1
fi

TEMP_DIR="${ARCHIVER_DIR}/temp_import"
mkdir -p "${TEMP_DIR}"
tar -xf "${TEMP_TAR}" -C "${TEMP_DIR}"
if [ $? -ne 0 ]; then
  echo "Error: Extraction failed. Please check the tarball and try again."
  rm -f "${TEMP_TAR}"
  rm -rf "${TEMP_DIR}"
  exit 1
fi

rm -f "${TEMP_TAR}"

mv "${TEMP_DIR}/config.sh" "${CONFIG_FILE}"
mkdir -p "${KEYS_DIR}"
mv "${TEMP_DIR}/keys"/* "${KEYS_DIR}/"
chmod 700 "${KEYS_DIR}"
# The RSA keypair is always present; the SSH keypair is optional (only sftp targets need it,
# and an env-native bundle may not carry it), so chmod only the key files that exist.
[ -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ] && chmod 600 "${DUPLICACY_RSA_PRIVATE_KEY_FILE}"
[ -f "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" ] && chmod 644 "${DUPLICACY_RSA_PUBLIC_KEY_FILE}"
[ -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ] && chmod 600 "${DUPLICACY_SSH_PRIVATE_KEY_FILE}"
[ -f "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" ] && chmod 644 "${DUPLICACY_SSH_PUBLIC_KEY_FILE}"
chmod 600 "${CONFIG_FILE}"

rm -rf "${TEMP_DIR}"

if [ ! -f "${CONFIG_FILE}" ]; then
  echo "Error: Configuration file was not created during import."
  exit 1
fi

if [ ! -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ] || [ ! -f "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" ]; then
  echo "Error: RSA key files were not created during import."
  exit 1
fi

echo "Import completed successfully."
