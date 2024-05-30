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

# Ensure the script is run as root
if [ "$(id -u)" -ne 0 ]; then
  echo "This script must be run as root. Please use sudo or log in as the root user." 1>&2
  exit 1
fi

# Configuration Section
# ---------------------

ARCHIVER_DIR="$(dirname "$(readlink -f "$0")")" # Path to Archiver directory
DUPLICACY_VERSION="3.2.3"
DUPLICACY_KEYS_DIR="${ARCHIVER_DIR}/.keys"
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

mkdir -p "${DUPLICACY_KEYS_DIR}"
RSA_PASSPHRASE=""

# Ensure necessary packages are installed
install_packages() {
  local missing_packages

  missing_packages=()

  # Check for each required package
  for package in "${REQUIRED_PACKAGES[@]}"; do
    if ! dpkg -l | grep -q "^ii  $package "; then
      missing_packages+=("$package")
    fi
  done

  # If there are no missing packages, exit the function
  if [ ${#missing_packages[@]} -eq 0 ]; then
    echo "All required packages are already installed."
    return
  fi

  # List missing packages and prompt user for installation
  echo "The following required packages are missing: ${missing_packages[*]}"
  read -p "Would you like to install the missing packages? (y/N): " -n 1 -r
  echo    # Move to a new line

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Exiting the script. Please install the required packages manually."
    exit 1
  fi

  # Update apt and install missing packages
  echo "Updating package list and installing missing packages..."
  apt update
  apt install -y "${missing_packages[@]}"
  echo "Missing packages installed successfully."
}

install_duplicacy() {
  echo    # Move to a new line
  read -p "Would you like to install the Duplicacy binary for use with Archiver? (y|N):" -n 1 -r
  echo    # Move to a new line
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Installing Duplicacy binary..."
    mkdir -p "${DUPLICACY_BIN_FILE_DIR}"
    wget -O "${DUPLICACY_BIN_FILE_PATH}" "${DUPLICACY_BIN_URL}"
    chmod 755 "${DUPLICACY_BIN_FILE_PATH}"
    mkdir -p "${DUPLICACY_BIN_LINK_DIR}"
    ln -sf "${DUPLICACY_BIN_FILE_PATH}" "${DUPLICACY_BIN_LINK_PATH}"
    echo "Duplicacy binary installed successfully."
  else
    echo "Duplicacy binary not installed. Please ensure Duplicacy is installed before attempting to run Archiver script."
  fi
}

backup_existing_file() {
  local file_path="$1"
  if [ -f "$file_path" ]; then
    local backup_path="${file_path}.backup"
    mv "$file_path" "$backup_path"
    echo "Existing $file_path backed up to $backup_path"
  fi
}

generate_rsa_keypair() {
  if [ ! -f "${DUPLICACY_KEYS_DIR}/private.pem" ] || [ ! -f "${DUPLICACY_KEYS_DIR}/public.pem" ]; then
    echo    # Move to a new line
    read -p "Would you like to generate an RSA key pair for Duplicacy encryption? (y|N):" -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo "Generating RSA key pair for Duplicacy encryption..."

      backup_existing_file "${DUPLICACY_KEYS_DIR}/private.pem"
      backup_existing_file "${DUPLICACY_KEYS_DIR}/public.pem"

      while [ -z "${RSA_PASSPHRASE}" ]; do
        # Please provide an RSA Passphrase to use with this new RSA key pair
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
      echo "RSA key pair generated successfully."
    else
      echo "RSA key pair not generated. Please provide your own, and copy them to archiver/.keys/private.pem and archiver/.keys/public.pem"
    fi
  else
    echo "Skipping RSA key pair generation: RSA key files already present in .keys directory."
  fi
}

generate_ssh_keypair() {
  if [ ! -f "${DUPLICACY_KEYS_DIR}/id_ed25519" ] || [ ! -f "${DUPLICACY_KEYS_DIR}/id_ed25519.pub" ]; then
    echo    # Move to a new line
    read -p "Would you like to generate an SSH key pair for Duplicacy SFTP storage? (y|N):" -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo "Generating SSH key pair for Duplicacy SFTP storage..."
      backup_existing_file "${DUPLICACY_KEYS_DIR}/id_ed25519"
      backup_existing_file "${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      ssh-keygen -t ed25519 -f "${DUPLICACY_KEYS_DIR}/id_ed25519" -N "" -C "archiver"
      chown -R "${CALLER_UID}:${CALLER_GID}" "${DUPLICACY_KEYS_DIR}"
      chmod 700 "${DUPLICACY_KEYS_DIR}"
      chmod 600 "${DUPLICACY_KEYS_DIR}/id_ed25519"
      chmod 644 "${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      echo "SSH key pair generated successfully."
    else
      echo "SSH key pair not generated. Please provide your own, and copy them to archiver/.keys/id_ed25519 and archiver/.keys/id_ed25519.pub"
    fi
  else
    echo "Skipping SSH key pair generation: SSH key files already present in .keys directory."
  fi
}

create_config_file() {
  echo    # Move to a new line
  read -p "Would you like to generate your config.sh file now? (y|N):" -n 1 -r
  echo    # Move to a new line
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Creating config.sh file..."
    backup_existing_file "${ARCHIVER_DIR}/config.sh"

    # Prompt user for SERVICE_DIRECTORIES
    echo "Enter the service directories you would like to backup (comma-separated, e.g., /srv/*/,/mnt/*/,/home/user/):"
    read -r service_directories_input
    IFS=',' read -r -a service_directories <<< "$service_directories_input"

    # Prompt user for Duplicacy security details
    echo "Enter security details for Duplicacy access and encryption:"
    echo "Create if this is a new install, or provide prior details if restoring:"

    while [ -z "${storage_password}" ]; do
      echo    # Move to a new line
      read -rsp "Storage Password (required): " storage_password
      echo    # Move to a new line
      if [ -z "${storage_password}" ]; then
        echo "Error: Storage Password is required."
      fi
    done

    while [ -z "${RSA_PASSPHRASE}" ]; do
      echo    # Move to a new line
      read -rsp "RSA Passphrase (required): " RSA_PASSPHRASE
      echo    # Move to a new line
      if [ -z "${RSA_PASSPHRASE}" ]; then
        echo "Error: RSA Passphrase is required."
      fi
    done

    echo    # Move to a new line
    read -p "Would you like to setup Pushover notifications? (y|N):" -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      # Prompt user for Pushover Notifications details
      echo "Enter Pushover notification details:"
      notification_service="Pushover"
      read -rp "Pushover User Key: " pushover_user_key
      read -rp "Pushover API Token: " pushover_api_token
    else
      notification_service="None"
      pushover_user_key=""
      pushover_api_token=""
    fi

    # Function to prompt for SFTP storage details
    prompt_sftp_storage() {
      read -rp "SFTP URL (ip address or fqdn of sftp host): " sftp_url
      read -rp "SFTP PORT (sftp port of host - default is 22): " sftp_port
      sftp_port=${sftp_port:-22}
      read -rp "SFTP User: " sftp_user
      read -rp "SFTP Path (directory path on sftp host): " sftp_path
      sftp_path=$(echo "$sftp_path" | sed 's|^/*||;s|/*$||')
      sftp_key_file="${DUPLICACY_KEYS_DIR}/id_ed25519"
    }

    # Function to prompt for B2 storage details
    prompt_b2_storage() {
      read -rp "B2 Bucket Name: " b2_bucketname
      read -rp "B2 ID (keyID from BackBlaze): " b2_id
      read -rp "B2 Key (applicationKey from BackBlaze): " b2_key
    }

    # Start writing the config file
    cat <<EOL > "${ARCHIVER_DIR}/config.sh"
# Archiver Backup Configuration

SERVICE_DIRECTORIES=(
$(for dir in "${service_directories[@]}"; do echo "  \"$dir\""; done)
)

EOL

    # Prompt user for storage targets
    i=1
    while true; do
      echo    # Move to a new line
      read -p "Would you like to add a(nother) storage target? (y|N): " -n 1 -r
      echo    # Move to a new line
      if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        break
      fi

      echo "Enter details for STORAGE_TARGET_$i:"
      read -rp "Name (you can call this whatever you want, but it must be unique): " name
      while true; do
        read -rp "Type (sftp/b2): " type
        if [[ $type == "sftp" ]]; then
          prompt_sftp_storage
          break
        elif [[ $type == "b2" ]]; then
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
    done

    # Write the rest of the config file
    cat <<EOL >> "${ARCHIVER_DIR}/config.sh"
# Example SFTP Storage Target
  # STORAGE_TARGET_1_NAME="name"
  # STORAGE_TARGET_1_TYPE="type"
  # STORAGE_TARGET_1_SFTP_URL="192.168.1.1"
  # STORAGE_TARGET_1_SFTP_PORT="22"
  # STORAGE_TARGET_1_SFTP_USER="user"
  # STORAGE_TARGET_1_SFTP_PATH="remote/path"
  # STORAGE_TARGET_1_SFTP_KEY_FILE="/path/to/id_ed25519"

# Example B2 Storage Target
  # STORAGE_TARGET_2_NAME="name"
  # STORAGE_TARGET_2_TYPE="type"
  # STORAGE_TARGET_2_B2_BUCKETNAME="bucketName"
  # STORAGE_TARGET_2_B2_ID="keyID"
  # STORAGE_TARGET_2_B2_KEY="applicationKey"

# Secrets for all Duplicacy storage targets
STORAGE_PASSWORD="${storage_password}" # Password for Duplicacy storage
RSA_PASSPHRASE="${RSA_PASSPHRASE}" # Passphrase for RSA private key

# Pushover Notifications
NOTIFICATION_SERVICE="$notification_service" # Currently support 'None' or 'Pushover'
PUSHOVER_USER_KEY="$pushover_user_key" # Pushover user key (not email address), viewable when logged into Pushover dashboard
PUSHOVER_API_TOKEN="$pushover_api_token" # Pushover application API token/key
EOL

    chown "${CALLER_UID}:${CALLER_GID}" "${ARCHIVER_DIR}/config.sh"
    chmod 600 "${ARCHIVER_DIR}/config.sh"
    echo "Configuration file created at ${ARCHIVER_DIR}/config.sh"
  else
    echo "Configuration file generation skipped."
  fi
}

schedule_with_cron() {
  echo    # Move to a new line
  read -p "Would you like to schedule the script with cron? (y|N):" -n 1 -r
  echo    # Move to a new line
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    (sudo crontab -l 2>/dev/null; echo "0 3 * * * ${ARCHIVER_DIR}/main.sh") | sudo crontab -
    echo "Added to crontab."
  else
    echo "Not added. You can always add it later with this command:"
    echo "--------------------------------------------"
    echo "(sudo crontab -l 2>/dev/null; echo \"0 3 * * * ${ARCHIVER_DIR}/main.sh\") | sudo crontab -"
    echo "--------------------------------------------"
  fi
}

main() {
  install_packages

  install_duplicacy

  generate_rsa_keypair

  generate_ssh_keypair

  create_config_file

  schedule_with_cron

  echo "Installation completed."
  echo "Please keep a separate backup of your config.sh file and your .keys directory."
  echo "To manually run the Archiver script, use 'sudo ./main.sh' from the archiver directory."
}

main
