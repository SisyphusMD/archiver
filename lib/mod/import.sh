#!/bin/bash

# Determine archiver repo directory path by traversing up the directory tree until we find 'archiver.sh' or reach the root
IMPORT_SCRIPT_PATH="$(realpath "$0")"
CURRENT_DIR="$(dirname "${IMPORT_SCRIPT_PATH}")"
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

# Get the UID and GID of the user who invoked the script
CALLER_UID=$(id -u "${SUDO_USER}")
CALLER_GID=$(id -g "${SUDO_USER}")

# Find export files in the archiver directory
mapfile -t EXPORT_FILES < <(ls -t "${ARCHIVER_DIR}"/export-*.tar.enc 2>/dev/null)

# Check if there are any export files
if [ ${#EXPORT_FILES[@]} -eq 0 ]; then
  echo "No export files found in ${ARCHIVER_DIR}."
  exit 1
elif [ ${#EXPORT_FILES[@]} -eq 1 ]; then
  SELECTED_FILE="${EXPORT_FILES[0]}"
else
  echo "Multiple export files found. Please choose one to import:"
  select FILE in "${EXPORT_FILES[@]}"; do
    if [ -n "$FILE" ]; then
      SELECTED_FILE="$FILE"
      break
    else
      echo "Invalid selection. Please try again."
    fi
  done
fi

# Prompt for password to decrypt the selected file
echo "Enter password to decrypt the selected export file:"
read -rs PASSWORD

# Define temporary output tar file
TEMP_TAR="${SELECTED_FILE%.enc}"

# Decrypt the selected export file
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
  mkdir -p "${ARCHIVER_DIR}/keys.bckp"
  mv "${ARCHIVER_DIR}/keys"/* "${ARCHIVER_DIR}/keys.bckp/"
fi

# Move the extracted files to their original locations
mv "${TEMP_DIR}/config.sh" "${ARCHIVER_DIR}/config.sh"
mkdir -p "${ARCHIVER_DIR}/keys"
mv "${TEMP_DIR}/keys"/* "${ARCHIVER_DIR}/keys/"
# Set permissions and ownership
chown -R "${CALLER_UID}:${CALLER_GID}" "${ARCHIVER_DIR}/keys"
chmod 700 "${ARCHIVER_DIR}/keys"
chmod 600 "${ARCHIVER_DIR}/keys/private.pem"
chmod 644 "${ARCHIVER_DIR}/keys/public.pem"
chmod 600 "${ARCHIVER_DIR}/keys/id_ed25519"
chmod 644 "${ARCHIVER_DIR}/keys/id_ed25519.pub"
chown "${CALLER_UID}:${CALLER_GID}" "${ARCHIVER_DIR}/config.sh"
chmod 600 "${ARCHIVER_DIR}/config.sh"

# Clean up temporary directory
rm -rf "${TEMP_DIR}"

echo "Import completed successfully. Existing config.sh and keys have been backed up with .bckp suffix."
