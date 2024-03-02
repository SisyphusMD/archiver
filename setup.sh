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
DUPLICACY_BINARY_URL="https://github.com/gilbertchen/duplicacy/releases/download/v${DUPLICACY_VERSION}/duplicacy_${DUPLICACY_OS}_${DUPLICACY_ARCHITECTURE}_${DUPLICACY_VERSION}"

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

echo "${DUPLICACY_BINARY_URL}"
echo "Installation and setup completed successfully."
