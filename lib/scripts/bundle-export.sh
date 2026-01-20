#!/bin/bash

BUNDLE_EXPORT_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi

echo "This script will create a bundle file containing your config.sh and keys directory."
echo "The bundle will be encrypted and protected by a password you provide below."
echo "You must remember this password and keep a copy of the bundle file."

if [ -n "${BUNDLE_PASSWORD}" ]; then
    echo ""
    read -p "Reuse existing BUNDLE_PASSWORD environment variable? (Y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Nn]$ ]]; then
        PASSWORD="${BUNDLE_PASSWORD}"
        echo "Using existing password."
    fi
fi

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

mkdir -p "${BUNDLE_DIR}"
chmod 700 "${BUNDLE_DIR}"

OUTPUT_TAR="${BUNDLE_DIR}/bundle.tar"
OUTPUT_ENC="${OUTPUT_TAR}.enc"

if [ -f "${OUTPUT_ENC}" ]; then
  [ -f "${OUTPUT_ENC}.old" ] && rm -f "${OUTPUT_ENC}.old"

  mv "${OUTPUT_ENC}" "${OUTPUT_ENC}.old"

  if [ -f "${OUTPUT_ENC}.old" ] && [ ! -f "${OUTPUT_ENC}" ]; then
    echo "Existing bundle backed up as bundle.tar.enc.old"
  else
    echo "Error: Failed to backup existing bundle file"
    echo "The file may be locked or in use. Please ensure no other processes are accessing it."
    exit 1
  fi
fi

cd "${ARCHIVER_DIR}"
tar -cf "${OUTPUT_TAR}" keys config.sh

openssl enc -aes-256-cbc -pbkdf2 -salt -in "${OUTPUT_TAR}" -out "${OUTPUT_ENC}" -k "${PASSWORD}"

rm "${OUTPUT_TAR}"

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
