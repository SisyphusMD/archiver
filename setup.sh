#!/bin/bash
# ==============================================================================
# Installation Script for Archiver Service
# ==============================================================================
# This script automates the installation and initial setup required for running
# the Archiver service. It ensures that all necessary dependencies are
# installed and configurations are applied.
#
# Usage:
#   sudo ./setup.sh
#
# Pre-requisites:
# - The script must be run as root or with sudo privileges.
# - Internet connection for downloading necessary packages.
# - Compatible with linux-based systems.
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

set -e # Exit immediately if a command exits with a non-zero status

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

# Configuration Section
# ---------------------

# Determine the full path of the script
SETUP_SCRIPT_PATH="$(readlink -f "${0}" 2>/dev/null)"
# Determine the full path of the containing dir of the script
ARCHIVER_DIR="$(cd "$(dirname "${SETUP_SCRIPT_PATH}")" && pwd)"
ARCHIVER_SCRIPT_PATH="${ARCHIVER_DIR}/archiver.sh"
DUPLICACY_VERSION="3.2.3"
DUPLICACY_KEYS_DIR="${ARCHIVER_DIR}/keys"
REQUIRED_PACKAGES=(
  "expect"
  "openssh-client"
  "openssl"
  "wget"
)

# Determine environment
ENVIRONMENT_OS="$(uname -s)"
ENVIRONMENT_ARCHITECTURE="$(uname -m)"
DUPLICACY_OS="$(echo ${ENVIRONMENT_OS} | tr '[:upper:]' '[:lower:]')"
DUPLICACY_ARCHITECTURE="unknown"
if [[ "${ENVIRONMENT_ARCHITECTURE}" == "aarch64" || "${ENVIRONMENT_ARCHITECTURE}" == "arm64" ]]; then
  DUPLICACY_ARCHITECTURE="arm64"
elif [[ "${ENVIRONMENT_ARCHITECTURE}" == "x86_64" || "${ENVIRONMENT_ARCHITECTURE}" == "amd64" ]]; then
  DUPLICACY_ARCHITECTURE="x64"
fi
# Get the UID and GID of the user who invoked the script
CALLER_UID=$(id -u "${SUDO_USER}")
CALLER_GID=$(id -g "${SUDO_USER}")
# Get the home directory of the user who invoked the script
CALLER_HOME=$(eval echo ~"${SUDO_USER}")

# Exit if the operating system is not Linux or architecture is not recognized
if [ "${DUPLICACY_OS}" != "linux" ] || [ "${DUPLICACY_ARCHITECTURE}" = "unknown" ]; then
  echo "This script only works on Linux environments with arm64 or x64 architectures." 1>&2
  exit 1
fi

DUPLICACY_BIN_FILE_DIR="/opt/duplicacy"
DUPLICACY_BIN_FILE_NAME="duplicacy_${DUPLICACY_OS}_${DUPLICACY_ARCHITECTURE}_${DUPLICACY_VERSION}"
DUPLICACY_BIN_FILE_PATH="${DUPLICACY_BIN_FILE_DIR}/${DUPLICACY_BIN_FILE_NAME}"
DUPLICACY_BIN_LINK_DIR="/usr/local/bin"
DUPLICACY_BIN_LINK_NAME="duplicacy"
DUPLICACY_BIN_LINK_PATH="${DUPLICACY_BIN_LINK_DIR}/${DUPLICACY_BIN_LINK_NAME}"
DUPLICACY_BIN_URL="https://github.com/gilbertchen/duplicacy/releases/download/v${DUPLICACY_VERSION}/${DUPLICACY_BIN_FILE_NAME}"

if [[ ! -d "${DUPLICACY_KEYS_DIR}" ]]; then
  mkdir -p "${DUPLICACY_KEYS_DIR}"
  chown -R "${CALLER_UID}:${CALLER_GID}" "${DUPLICACY_KEYS_DIR}"
  chmod 700 "${DUPLICACY_KEYS_DIR}"
fi

RSA_PASSPHRASE=""

# Place archiver in PATH
archiver_in_path() {
  local symlink_path
  symlink_path="/usr/local/bin/archiver"

  if [ ! -L "${symlink_path}" ]; then
    sudo ln -s "${ARCHIVER_SCRIPT_PATH}" "${symlink_path}"
    echo " - Added 'archiver' to PATH."
  else
    local existing_target
    existing_target="$(readlink -f "${symlink_path}")"

    if [ "${existing_target}" != "${ARCHIVER_SCRIPT_PATH}" ]; then
      echo    # Move to a new line
      echo "Another archiver symlink exists but points to a different script: '${existing_target}'."
      read -p "Would you like to update it to point to '${ARCHIVER_SCRIPT_PATH}'? (y/N): " -n 1 -r
      echo    # Move to a new line

      if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo rm "${symlink_path}"
        sudo ln -s "${ARCHIVER_SCRIPT_PATH}" "${symlink_path}"
        echo " - Symlink has been updated: ${symlink_path} -> ${ARCHIVER_SCRIPT_PATH}"
      else
        echo " - Symlink not updated."
      fi
    else
      echo " - Symlink in PATH already points to '${ARCHIVER_SCRIPT_PATH}' correctly."
    fi
  fi
}

# Ensure necessary packages are installed
install_packages() {
  local missing_packages

  missing_packages=()

  echo " - Checking for missing required packages..."

  # Check for each required package
  for package in "${REQUIRED_PACKAGES[@]}"; do
    if ! dpkg -l | grep -q "^ii  $package "; then
      missing_packages+=("$package")
    fi
  done

  # If there are no missing packages, exit the function
  if [ ${#missing_packages[@]} -eq 0 ]; then
    echo " - All required packages are already installed."
    return
  fi

  # List missing packages and prompt user for installation
  echo " - The following required packages are missing: ${missing_packages[*]}"
  echo    # Move to a new line
  echo    # Move to a new line
  read -p "Would you like to install the missing packages? (y/N): " -n 1 -r
  echo    # Move to a new line

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo " - Exiting the script. Please install the required packages manually."
    exit 1
  fi

  # Update apt and install missing packages
  echo " - Updating package list and installing missing packages..."
  apt update
  apt install -y "${missing_packages[@]}"
  echo " - Missing packages installed successfully."
}

install_duplicacy() {
  # Check if duplicacy is available and install if not
  if ! command -v duplicacy &> /dev/null; then
    echo    # Move to a new line
    echo    # Move to a new line
    echo "Duplicacy binary is required for Archiver, but it is not installed."
    echo    # Move to a new line
    read -p "Would you like to install the Duplicacy binary for use with Archiver? (y|N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo " - Installing Duplicacy binary..."
      mkdir -p "${DUPLICACY_BIN_FILE_DIR}"
      wget -O "${DUPLICACY_BIN_FILE_PATH}" "${DUPLICACY_BIN_URL}"
      chmod 755 "${DUPLICACY_BIN_FILE_PATH}"
      mkdir -p "${DUPLICACY_BIN_LINK_DIR}"
      ln -sf "${DUPLICACY_BIN_FILE_PATH}" "${DUPLICACY_BIN_LINK_PATH}"
      echo " - Duplicacy binary installed successfully."
    else
      echo " - Duplicacy binary not installed. Please ensure Duplicacy binary is installed before attempting to run the main script."
    fi
  else
    echo " - Skipping Duplicacy binary installation: Duplicacy binary is already installed."
  fi
}

backup_existing_file() {
  local file_path="$1"
  if [ -f "$file_path" ]; then
    local backup_path="${file_path}.backup"
    mv "$file_path" "$backup_path"
    echo " - Existing $file_path backed up to $backup_path"
  fi
}

generate_rsa_keypair() {
  if [ ! -f "${DUPLICACY_KEYS_DIR}/private.pem" ] || [ ! -f "${DUPLICACY_KEYS_DIR}/public.pem" ]; then
    echo    # Move to a new line
    echo    # Move to a new line
    read -p "Would you like to generate an RSA key pair for Duplicacy encryption? (y|N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo " - Generating RSA key pair for Duplicacy encryption..."

      backup_existing_file "${DUPLICACY_KEYS_DIR}/private.pem"
      backup_existing_file "${DUPLICACY_KEYS_DIR}/public.pem"

      while [ -z "${RSA_PASSPHRASE}" ]; do
        # Please provide an RSA Passphrase to use with this new RSA key pair
        echo    # Move to a new line
        read -rsp "RSA Passphrase (required): " RSA_PASSPHRASE
        echo    # Move to a new line
        if [ -z "${RSA_PASSPHRASE}" ]; then
          echo "Error: RSA Passphrase is required."
        fi
      done

      # Running expect scripts to handle the prompts
      expect <<EOF
spawn openssl genrsa -aes256 -out "${DUPLICACY_KEYS_DIR}/private.pem" -traditional 2048
expect "Enter PEM pass phrase:"
send "${RSA_PASSPHRASE}\r"
expect "Verifying - Enter PEM pass phrase:"
send "${RSA_PASSPHRASE}\r"
expect eof
EOF

      expect <<EOF
spawn openssl rsa -in "${DUPLICACY_KEYS_DIR}/private.pem" --outform PEM -pubout -out "${DUPLICACY_KEYS_DIR}/public.pem"
expect "Enter pass phrase for ${DUPLICACY_KEYS_DIR}/private.pem:"
send "${RSA_PASSPHRASE}\r"
expect eof
EOF

      chown -R "${CALLER_UID}:${CALLER_GID}" "${DUPLICACY_KEYS_DIR}"
      chmod 700 "${DUPLICACY_KEYS_DIR}"
      chmod 600 "${DUPLICACY_KEYS_DIR}/private.pem"
      chmod 644 "${DUPLICACY_KEYS_DIR}/public.pem"
      echo " - RSA key pair generated successfully."
    else
      echo " - RSA key pair not generated."
      echo " - Please provide your own, and copy them to ${DUPLICACY_KEYS_DIR}/private.pem and ${DUPLICACY_KEYS_DIR}/public.pem"
      echo " - Details at: https://forum.duplicacy.com/t/new-feature-rsa-encryption/2662"
    fi
  else
    echo " - Skipping RSA key pair generation: RSA key files already present in '${DUPLICACY_KEYS_DIR}'."
  fi
}

generate_ssh_keypair() {
  if [ ! -f "${DUPLICACY_KEYS_DIR}/id_ed25519" ] || [ ! -f "${DUPLICACY_KEYS_DIR}/id_ed25519.pub" ]; then
    echo    # Move to a new line
    echo    # Move to a new line
    read -p "Would you like to generate an SSH key pair for Duplicacy SFTP storage? (y|N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo " - Generating SSH key pair for Duplicacy SFTP storage..."
      backup_existing_file "${DUPLICACY_KEYS_DIR}/id_ed25519"
      backup_existing_file "${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      ssh-keygen -t ed25519 -f "${DUPLICACY_KEYS_DIR}/id_ed25519" -N "" -C "archiver"
      chown -R "${CALLER_UID}:${CALLER_GID}" "${DUPLICACY_KEYS_DIR}"
      chmod 700 "${DUPLICACY_KEYS_DIR}"
      chmod 600 "${DUPLICACY_KEYS_DIR}/id_ed25519"
      chmod 644 "${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      echo " - SSH key pair generated successfully."
    else
      echo " - SSH key pair not generated."
      echo " - Please provide your own, and copy them to ${DUPLICACY_KEYS_DIR}/id_ed25519 and ${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      echo " - Archiver only supports ed25519 key pairs with no passphrase for SFTP."
      echo " - Can use the following command: ssh-keygen -t ed25519 -f "${DUPLICACY_KEYS_DIR}/id_ed25519" -N "" -C \"archiver\""
    fi
  else
    echo " - Skipping SSH key pair generation: SSH key files already present in '${DUPLICACY_KEYS_DIR}'."
  fi
}

create_config_file() {
  if [ ! -f "${ARCHIVER_DIR}/config.sh" ]; then
    echo    # Move to a new line
    echo    # Move to a new line
    read -p "Would you like to generate your config.sh file now? (y|N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo " - Creating config.sh file..."
      backup_existing_file "${ARCHIVER_DIR}/config.sh"

      # Prompt user for SERVICE_DIRECTORIES
      echo    # Move to a new line
      echo "Please provide a list of directories on your device to be backed up. Must provide the"
      echo "  full paths. Can use * to indicate each individual subdirectory within the parent"
      echo "  directory. Each directory will be backed up as an individual duplicacy repository."

      while [ -z "${service_directories_input}" ]; do
        echo    # Move to a new line
        echo "Enter the service directories you would like to backup (comma-separated, e.g., /srv/*/,/mnt/*/,${CALLER_HOME}/):"
        read -r service_directories_input
        echo    # Move to a new line
        if [ -z "${service_directories_input}" ]; then
          echo " - Error: At least one service directory is required."
        fi
      done
      IFS=',' read -r -a service_directories <<< "$service_directories_input"

      # Prompt user for Duplicacy security details
      echo "Enter security details for Duplicacy access and encryption:"
      echo "Create this storage password and rsa passphrase if this is a new install, or provide prior details if restoring:"

      while [ -z "${storage_password}" ]; do
        echo    # Move to a new line
        read -rsp "Storage Password (required): " storage_password
        echo    # Move to a new line
        if [ -z "${storage_password}" ]; then
          echo " - Error: Storage Password is required."
        fi
      done

      while [ -z "${RSA_PASSPHRASE}" ]; do
        echo    # Move to a new line
        read -rsp "RSA Passphrase (required): " RSA_PASSPHRASE
        echo    # Move to a new line
        if [ -z "${RSA_PASSPHRASE}" ]; then
          echo " - Error: RSA Passphrase is required."
        fi
      done

      # Function to prompt for SFTP storage details
      prompt_sftp_storage() {
        sftp_url=""
        sftp_port=""
        sftp_user=""
        sftp_path=""

        while [ -z "${sftp_url}" ]; do
          echo    # Move to a new line
          read -rp "SFTP URL (The IP address or FQDN of the sftp host - example: 192.168.1.1): " sftp_url
          if [ -z "${sftp_url}" ]; then
            echo " - Error: SFTP URL is required."
          fi
        done

        while [ -z "${sftp_port}" ]; do
          echo    # Move to a new line
          read -rp "SFTP Port (The sftp port of the sftp host - default is 22): " sftp_port
          if [ -z "${sftp_port}" ]; then
            echo " - No port entered. Using default port 22."
            sftp_port=22
          fi
        done

        while [ -z "${sftp_user}" ]; do
          echo    # Move to a new line
          read -rp "SFTP User (User with sftp privileges on sftp host): " sftp_user
          if [ -z "${sftp_user}" ]; then
            echo " - Error: SFTP User is required."
          fi
        done

        while [ -z "${sftp_path}" ]; do
          echo    # Move to a new line
          read -rp "SFTP Path (Absolute path to remote backup directory - example: remote/path): " sftp_path
          if [ -z "${sftp_path}" ]; then
            echo " - Error: SFTP Path is required."
          fi
        done
        sftp_path="$(echo "${sftp_path}" | sed 's|^/*||;s|/*$||')"

        sftp_key_file="${DUPLICACY_KEYS_DIR}/id_ed25519"
      }

      # Function to prompt for B2 storage details
      prompt_b2_storage() {
        b2_bucketname=""
        b2_id=""
        b2_key=""

        while [ -z "${b2_bucketname}" ]; do
          echo    # Move to a new line
          read -rp "B2 Bucket Name (BackBlaze bucket name - must be globally unique): " b2_bucketname
          if [ -z "${b2_bucketname}" ]; then
            echo " - Error: B2 Bucket Name is required."
          fi
        done

        while [ -z "${b2_id}" ]; do
          echo    # Move to a new line
          read -rp "B2 keyID (BackBlaze keyID with read/write access to the bucket): " b2_id
          if [ -z "${b2_id}" ]; then
            echo " - Error: B2 keyID is required."
          fi
        done

        while [ -z "${b2_key}" ]; do
          echo    # Move to a new line
          read -rsp "B2 applicationKey (BackBlaze applicationKey with read/write access to the bucket): " b2_key
          echo    # Move to a new line
          if [ -z "${b2_key}" ]; then
            echo " - Error: B2 keyID is required."
          fi
        done
      }

      # Start writing the config file
      cat <<EOL > "${ARCHIVER_DIR}/config.sh"
#########################################################################################
# Archiver User Configuration                                                           #
#########################################################################################
# config.sh                                                                             #
#   This file is intended to be sourced by the Archiver script to define user           #
#   configurable variables.                                                             #
#                                                                                       #
# Usage:                                                                                #
#   Include this file as "config.sh" in the archiver directory                          #
#                                                                                       #
# Note:                                                                                 #
#   This script should not be executed directly. Instead, it will be sourced by the     #
#   Archiver script.                                                                    #
#                                                                                       #
# Instructions:                                                                         #
# - User must provide at least one service directory and one storage target.            #
# - User must provide the STORAGE_PASSWORD and RSA_PASSPHRASE to be used by Archiver.   #
# - User can optionally provide notification and backup rotation configurations.        #
#########################################################################################

# ------------------ #
# REQUIRED VARIABLES #
# ------------------ #
SERVICE_DIRECTORIES=(
$(for dir in "${service_directories[@]}"; do echo "  \"${dir}\""; done)
)

# Example SERVICE_DIRECTORIES
# Please provide a list of directories on your device to be backed up. Must provide the"
#   full paths. Can use * to indicate each individual subdirectory within the parent"
#   directory. Each directory will be backed up as an individual duplicacy repository."
  # SERVICE_DIRECTORIES=(
  #   "/srv/*/" # Will backup each subdirectory within /srv/ - multiple individual repositories.
  #   "/mnt/*/" # Will backup each subdirectory within /mnt/ - multiple individual repositories.
  #   "${CALLER_HOME}/" # Will backup the ${CALLER_HOME}/ directory - one individual repository.
  # )

EOL

      # Prompt user for storage targets
      echo    # Move to a new line
      echo "Add primary storage target. (Configuration of first storage target is required)"
      i=1
      while true; do
        echo    # Move to a new line
        echo "Enter details for STORAGE_TARGET_$i:"

        name=""
        type=""

        while [ -z "${name}" ]; do
          echo    # Move to a new line
          read -rp "Storage Name (You can call this whatever you want, but it must be unique): " name
          if [ -z "${name}" ]; then
            echo "Error: Storage Name is required."
          fi
        done

        while true; do
          echo    # Move to a new line
          read -rp "Storage Type (Currently support sftp and b2): " type
          if [[ "${type}" == "sftp" ]]; then
            prompt_sftp_storage
            break
          elif [[ "${type}" == "b2" ]]; then
            prompt_b2_storage
            break
          else
            echo "Unsupported storage type. Please enter either 'sftp' or 'b2'."
          fi
        done

        # Write storage target details to config file
        cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
STORAGE_TARGET_${i}_NAME="$name"
STORAGE_TARGET_${i}_TYPE="$type"
EOL
        if [[ $type == "sftp" ]]; then
          cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
STORAGE_TARGET_${i}_SFTP_URL="$sftp_url"
STORAGE_TARGET_${i}_SFTP_PORT="$sftp_port"
STORAGE_TARGET_${i}_SFTP_USER="$sftp_user"
STORAGE_TARGET_${i}_SFTP_PATH="$sftp_path"
STORAGE_TARGET_${i}_SFTP_KEY_FILE="$sftp_key_file"

EOL
        elif [[ $type == "b2" ]]; then
          cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
STORAGE_TARGET_${i}_B2_BUCKETNAME="$b2_bucketname"
STORAGE_TARGET_${i}_B2_ID="$b2_id"
STORAGE_TARGET_${i}_B2_KEY="$b2_key"

EOL
        fi

        ((i++))
        echo    # Move to a new line
        read -p "Would you like to add another storage target? (y|N): " -n 1 -r
        echo    # Move to a new line
        if [[ ! "${REPLY}" =~ ^[Yy]$ ]]; then
          break
        fi
      done

      # Write more of the config file
      cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
# Storage targets must be numbered sequentially, starting with 1, following the naming
#   scheme STORAGE_TARGET_X_OPTION="config", with all options for the same storage
#   using the same X number, as in the below examples. SFTP and BackBlaze storage
#   targets are currently supported. Require at least one storage target.

# Example SFTP Storage Target
  # STORAGE_TARGET_1_NAME="name" # You can call this whatever you want, but it must be unique.
  # STORAGE_TARGET_1_TYPE="sftp" # Currently support sftp and b2. For sftp, require URL, PORT, USER, PATH, and KEY_FILE as below.
  # STORAGE_TARGET_1_SFTP_URL="192.168.1.1" # The IP address or FQDN of the sftp host.
  # STORAGE_TARGET_1_SFTP_PORT="22" # The sftp port of the sftp host. Default is 22.
  # STORAGE_TARGET_1_SFTP_USER="user" # User with sftp privileges on sftp host.
  # STORAGE_TARGET_1_SFTP_PATH="remote/path" # Absolute path to remote backup directory. For synology, this starts with the name of the shared folder.
  # STORAGE_TARGET_1_SFTP_KEY_FILE="/path/to/id_ed25519" # Full path to private ssh key file.

# Example B2 Storage Target
  # STORAGE_TARGET_2_NAME="name" # You can call this whatever you want, but it must be unique.
  # STORAGE_TARGET_2_TYPE="b2" # Currently support sftp and b2. For b2, require BUCKETNAME, ID, and KEY as below.
  # STORAGE_TARGET_2_B2_BUCKETNAME="bucketName" # BackBlaze bucket name. Must be globally unique.
  # STORAGE_TARGET_2_B2_ID="keyID" # BackBlaze keyID with read/write access to the above bucket.
  # STORAGE_TARGET_2_B2_KEY="applicationKey"  # BackBlaze applicationKey with read/write access to the above bucket.

# Secrets for all Duplicacy storage targets
STORAGE_PASSWORD="${storage_password}" # Password for Duplicacy storage (required)
RSA_PASSPHRASE="${RSA_PASSPHRASE}" # Passphrase for RSA private key (required)


EOL

      echo    # Move to a new line
      read -p "Would you like to setup Pushover notifications? (y|N):" -n 1 -r
      echo    # Move to a new line
      if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Prompt user for Pushover Notifications details
        echo    # Move to a new line
        echo "Enter Pushover notification details:"
        notification_service="Pushover"

        while [ -z "${pushover_user_key}" ]; do
          echo    # Move to a new line
          read -rp "Pushover User Key: " pushover_user_key
          if [ -z "${pushover_user_key}" ]; then
            echo "Error: Pushover User Key is required."
          fi
        done

        while [ -z "${pushover_api_token}" ]; do
          echo    # Move to a new line
          read -rsp "Pushover API Token: " pushover_api_token
          echo    # Move to a new line
          if [ -z "${pushover_api_token}" ]; then
            echo "Error: Pushover API Token is required."
          fi
        done
      else
        echo " - Pushover notifications not set up."
        echo " - They can be added manually later by editing config.sh."
        notification_service="None"
        pushover_user_key=""
        pushover_api_token=""
      fi

      # Write more of the config file
      cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
# ------------------ #
# OPTIONAL VARIABLES #
# ------------------ #
# Notifications
NOTIFICATION_SERVICE="$notification_service" # Currently support 'None' or 'Pushover'
PUSHOVER_USER_KEY="$pushover_user_key" # Pushover user key (not email address), viewable when logged into Pushover dashboard
PUSHOVER_API_TOKEN="$pushover_api_token" # Pushover application API token/key

EOL

      echo    # Move to a new line
      echo "By default, Archiver runs a Duplicacy prune operation at the end of every run to rotate backups."
      read -p "Would you like to disable rotating backups? (y|N):" -n 1 -r
      echo    # Move to a new line
      if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo    # Move to a new line
        rotate_backups="false"
        prune_keep=""
        echo "Backup rotations disabled. You can change this by editing your config.sh."
      else
        rotate_backups="true"
        echo "Backup rotations enabled. You can change this by editing your config.sh."
        echo    # Move to a new line
        echo    # Move to a new line
        echo "By default, Archiver's backup rotation schedule is as follows:"
        echo "  - Keep all backups made in the past 1 day."
        echo "  - Keep 1 backup per 1 day for backups older than 1 day."
        echo "  - Keep 1 backup per 7 days for backups older than 7 days."
        echo "  - Keep 1 backup per 30 days for backups older than 30 days."
        echo "  - Discard all backups older than 180 days."
        echo "Archiver never deletes the only remaining backup, even if it would be deleted otherwise according to the time criteria."
        echo "The above schedule is achieved with the following configuration:"
        echo "----------------"
        echo "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
        echo "----------------"
        echo "See https://forum.duplicacy.com/t/prune-command-details/1005 for details."
        echo    # Move to a new line
        read -p "Would you like to change from the default backup rotation schedule? (y|N):" -n 1 -r
        echo    # Move to a new line
        if [[ $REPLY =~ ^[Yy]$ ]]; then
          echo    # Move to a new line
          read -rp "Desired backup rotation schedule (press <return> for default): " prune_keep
          echo    # Move to a new line
          if [ -z "${prune_keep}" ]; then
            prune_keep="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
            echo " - No backup rotation schedule entered."
            echo " - Will use default backup rotation schedule: ${prune_keep}"
            echo "You can change this by editing your config.sh."
          else
            echo "Will use backup rotation schedule: ${prune_keep}"
            echo "You can change this by editing your config.sh."
          fi
        else
          prune_keep="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
          echo "Will use default backup rotation schedule: ${prune_keep}"
          echo "You can change this by editing your config.sh."
        fi
      fi

      # Write rest of the config file
      cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
# Backup Rotation
ROTATE_BACKUPS="${rotate_backups}" # Default: "true". Set to 'true' to enable rotating out older backups.
PRUNE_KEEP="${prune_keep}" # Default: "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1". See https://forum.duplicacy.com/t/prune-command-details/1005 for details.
EOL

      chown "${CALLER_UID}:${CALLER_GID}" "${ARCHIVER_DIR}/config.sh"
      chmod 600 "${ARCHIVER_DIR}/config.sh"
      echo " - Configuration file created at ${ARCHIVER_DIR}/config.sh"
    else
      echo " - Configuration file generation skipped."
      echo " - If you would like to create your config.sh file manually, you can"
      echo " -   copy config.sh.example from the examples directory to config.sh"
      echo " -   and place it in the parent archiver directory."
    fi
  else
    echo " - Skipping configuration file creation: Configuration file already present in archiver directory."
  fi
}

schedule_with_cron() {
  echo    # Move to a new line
  echo    # Move to a new line
  read -p "Would you like to schedule the backup with cron? (y|N): " -n 1 -r
  echo    # Move to a new line
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    local cron_schedule
    while [ -z "${cron_schedule}" ]; do
      echo    # Move to a new line
      echo "See guide for cron scheduling here: https://cronitor.io/guides/cron-jobs"
      read -rp "Cron Schedule (default to daily at 3am '0 3 * * *'): " cron_schedule
      if [ -z "${cron_schedule}" ]; then
        echo " - No schedule entered. Using default '0 3 * * *'."
        cron_schedule="0 3 * * *"
      fi
    done
    (sudo crontab -l 2>/dev/null; echo "${cron_schedule} archiver") | sudo crontab -
    echo " - Backup scheduled with cron."
    echo " - You can edit the schedule with this command:"
    echo "--------------------------------------------"
    echo "sudo crontab -e"
    echo "--------------------------------------------"
  else
    echo " - Backup not scheduled with cron."
    echo " - You can always schedule it later with this command (daily at 3am in below example):"
    echo "--------------------------------------------"
    echo "(sudo crontab -l 2>/dev/null; echo \"0 3 * * * archiver\") | sudo crontab -"
    echo "--------------------------------------------"
  fi
}

main() {
  archiver_in_path

  install_packages

  install_duplicacy

  generate_rsa_keypair

  generate_ssh_keypair

  create_config_file

  schedule_with_cron

  sleep 2

  echo    # Move to a new line
  echo " - Setup script completed."
  echo "IMPORTANT: You MUST keep a separate backup of your config.sh file and your keys directory."
  echo "Usage:"
  echo " - To manually start the Archiver backup, use 'archiver'."
  echo " - To watch the logs of the actively running Archiver backup, use 'archiver logs'. Will start Archiver backup if not already running."
  echo " - To manually stop the Archiver backup early, use 'archiver stop'."
}

main
