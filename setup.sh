#!/bin/bash
# ==============================================================================
# Installation Script for Archiver Service
# ==============================================================================
# This script automates the installation and initial setup required for running
# the Archiver service. It ensures that all necessary dependencies are
# installed, configurations are applied, and the system is prepared for the
# service to run efficiently.
#
# Usage:
#   sudo ./setup.sh
#
# Pre-requisites:
# - The script must be run as root or with sudo privileges.
# - Internet connection for downloading necessary packages.
# - Compatible with Debian-based systems.
#
# The script will:
# - Update the system package list.
# - Install required software packages and dependencies.
# - Configure system parameters and environment variables.
# - Download and set up any necessary scripts or binaries for the service.
#
# Please ensure you have read and understood the service's documentation
# before proceeding with the installation.
# ==============================================================================

# Configuration Section
# ---------------------

# Determine environment
ENVIRONMENT_OS="$(uname -s)"
ENVIRONMENT_ARCHITECTURE="$(uname -m)"
ARCHIVER_DIR="$(dirname "$(readlink -f "$0")")" # Path to Archiver directory
REQUIRED_PACKAGES=(
  "wget"
  "openssl"
  "sqlite3"
)

# Configuration for Duplicacy binary
DUPLICACY_VERSION="3.2.3"
DUPLICACY_OS="$(echo ${ENVIRONMENT_OS} | tr '[:upper:]' '[:lower:]')"
DUPLICACY_ARCHITECTURE=$( \
  if [[ "${ENVIRONMENT_ARCHITECTURE}" == "aarch64" || \
    "${ENVIRONMENT_ARCHITECTURE}" == "arm64" ]]; then \
  echo "arm64"; \
  elif [[ "${ENVIRONMENT_ARCHITECTURE}" == "x86_64" || \
    "${ENVIRONMENT_ARCHITECTURE}" == "amd64" ]]; then \
  echo "x64"; \
  else \
    echo "unknown"; \
  fi \
)
DUPLICACY_BIN_FILE_DIR="/opt/duplicacy" # Path to Duplicacy binary file directory
DUPLICACY_BIN_FILE_NAME="duplicacy_${DUPLICACY_OS}_${DUPLICACY_ARCHITECTURE}_${DUPLICACY_VERSION}" # Duplicacy binary file name
DUPLICACY_BIN_FILE_PATH="${DUPLICACY_BIN_FILE_DIR}/${DUPLICACY_BIN_FILE_NAME}" # Path to duplicacy binary file
DUPLICACY_BIN_LINK_DIR="/usr/local/bin" # Path to Duplicacy binary link directory
DUPLICACY_BIN_LINK_NAME="duplicacy" # Duplicacy binary link name
DUPLICACY_BIN_LINK_PATH="${DUPLICACY_BIN_LINK_DIR}/${DUPLICACY_BIN_LINK_NAME}" # Path to duplicacy binary link
DUPLICACY_BIN_URL="https://github.com/gilbertchen/duplicacy/releases/download/v${DUPLICACY_VERSION}/${DUPLICACY_BIN_FILE_NAME}" # Download URL for appropriate Duplicacy binary
DUPLICACY_KEYS_DIR="${ARCHIVER_DIR}/.keys"

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user." 1>&2
 exit 1
fi

# Exit if the operating system is not Linux
if [ "${DUPLICACY_OS}" != "linux" ]; then
  echo "This script only works in Linux environments. Please run this script on a Linux system." 1>&2
  exit 1
fi

# Exit if the architecture is not recognized as arm64 or x64
if [ "${DUPLICACY_OS}" = "unknown" ]; then
  echo "This script only works on arm64 and x64 architectures." 1>&2
  exit 1
fi

# [The rest of the script will go here]

# Attempt to detect package manager and install necessary packages
if command -v apt-get &> /dev/null; then
  echo "Attempting to install necessary packages using apt-get..."
  apt-get update
  apt-get install -y "${REQUIRED_PACKAGES[@]}"
elif command -v yum &> /dev/null; then
  echo "Attempting to install necessary packages using yum..."
  yum install -y "${REQUIRED_PACKAGES[@]}"
elif command -v dnf &> /dev/null; then
  echo "Attempting to install necessary packages using dnf..."
  dnf install -y "${REQUIRED_PACKAGES[@]}"
else
  echo "Package manager not detected. Please manually install the necessary packages."
  exit 1
fi

mkdir -p "${DUPLICACY_BIN_FILE_DIR}"
wget -O "${DUPLICACY_BIN_FILE_PATH}" "${DUPLICACY_BIN_URL}"
chmod 755 "${DUPLICACY_BIN_FILE_PATH}"
mkdir -p "${DUPLICACY_BIN_LINK_DIR}"
ln -sf "${DUPLICACY_BIN_FILE_PATH}" "${DUPLICACY_BIN_LINK_PATH}"
echo    # Move to a new line
read -p "You must provide a passphrase for the generated key or this command will error. Are you ready to provide a passphrase?" -n 1 -r
echo    # Move to a new line
if [[ $REPLY =~ ^[Yy]$ ]]; then
  mkdir -p "${DUPLICACY_KEYS_DIR}"
  openssl genrsa -aes256 -out "${DUPLICACY_KEYS_DIR}/private.pem" -traditional 2048
  openssl rsa -in "${DUPLICACY_KEYS_DIR}/private.pem" --outform PEM -pubout -out "${DUPLICACY_KEYS_DIR}/public.pem"
  ssh-keygen -f "${DUPLICACY_KEYS_DIR}/id_rsa" -N "" -C "archiver"
  chown -R 1000:1000 "${DUPLICACY_KEYS_DIR}"
  chmod 700 "${DUPLICACY_KEYS_DIR}"
  chmod 600 "${DUPLICACY_KEYS_DIR}/private.pem" "${DUPLICACY_KEYS_DIR}/id_rsa"
  chmod 644 "${DUPLICACY_KEYS_DIR}/public.pem" "${DUPLICACY_KEYS_DIR}/id_rsa.pub"
fi

# To add script to cron schedule:
echo    # Move to a new line
read -p "Would you like to schedule the script with cron?" -n 1 -r
echo    # Move to a new line
if [[ $REPLY =~ ^[Yy]$ ]]; then
  (crontab -l 2>/dev/null; echo "0 4 * * * ${ARCHIVER_DIR}/main.sh") | crontab -
  echo "Added"
else
  echo "Not added. You can always add it later with this command: (crontab -l 2>/dev/null; echo \"0 4 * * * ${ARCHIVER_DIR}/main.sh\") | crontab -"
fi


echo "Installation and setup completed successfully."
echo "If restoring, you must replace the contents of the generated key files in the .keys directory as needed."
echo "To manually run the script, use ./main.sh from the archiver directory."
