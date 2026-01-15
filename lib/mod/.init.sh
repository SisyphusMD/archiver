#!/bin/bash
#
# Archiver Initialization Script
# Creates configuration bundle for Docker deployment
#

set -e

# Require Docker environment
source "/opt/archiver/lib/src/require-docker.sh"

# Configuration Section
# ---------------------
# Archiver directory
ARCHIVER_DIR="/opt/archiver"
DUPLICACY_KEYS_DIR="${ARCHIVER_DIR}/keys"
BUNDLE_IMPORT_SCRIPT="${ARCHIVER_DIR}/lib/mod/bundle-import.sh"
BUNDLE_EXPORT_SCRIPT="${ARCHIVER_DIR}/lib/mod/bundle-export.sh"
BUNDLE_DIR="${ARCHIVER_DIR}/bundle"

RSA_PASSPHRASE=""


import_if_missing() {
  if [ ! -f "${DUPLICACY_KEYS_DIR}/private.pem" ] || [ ! -f "${DUPLICACY_KEYS_DIR}/public.pem" ] || \
    [ ! -f "${DUPLICACY_KEYS_DIR}/id_ed25519" ] || [ ! -f "${DUPLICACY_KEYS_DIR}/id_ed25519.pub" ] ||\
    [ ! -f "${ARCHIVER_DIR}/config.sh" ]; then

      # Check for bundle file
      if [ -f "${ARCHIVER_DIR}/bundle.tar.enc" ] || [ -f "${BUNDLE_DIR}/bundle.tar.enc" ]; then
        "${BUNDLE_IMPORT_SCRIPT}"
      else
        echo "No bundle file found. Skipping import and continuing fresh setup."
      fi

  fi
}

create_keys_dir() {
  if [[ ! -d "${DUPLICACY_KEYS_DIR}" ]]; then
    mkdir -p "${DUPLICACY_KEYS_DIR}"
    chmod 700 "${DUPLICACY_KEYS_DIR}"
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
      chmod 700 "${DUPLICACY_KEYS_DIR}"
      chmod 600 "${DUPLICACY_KEYS_DIR}/id_ed25519"
      chmod 644 "${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      echo " - SSH key pair generated successfully."
    else
      echo " - SSH key pair not generated."
      echo " - Please provide your own, and copy them to ${DUPLICACY_KEYS_DIR}/id_ed25519 and ${DUPLICACY_KEYS_DIR}/id_ed25519.pub"
      echo " - Archiver only supports ed25519 key pairs with no passphrase for SFTP."
      echo " - Can use the following command: ssh-keygen -t ed25519 -f ${DUPLICACY_KEYS_DIR}/id_ed25519 -N \"\" -C archiver"
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

      # Function to prompt for S3 storage details
      prompt_s3_storage() {
        s3_bucketname=""
        s3_endpoint=""
        s3_id=""
        s3_secret=""

        while [ -z "${s3_bucketname}" ]; do
          echo    # Move to a new line
          read -rp "S3 Bucket Name (Not the entire url, just the unique name of the bucket): " s3_bucketname
          if [ -z "${s3_bucketname}" ]; then
            echo " - Error: S3 Bucket Name is required."
          fi
        done

        while [ -z "${s3_endpoint}" ]; do
          echo    # Move to a new line
          read -rp "S3 Endpoint (ex: amazon.com or hel1.your-objectstorage.com): " s3_endpoint
          if [ -z "${s3_endpoint}" ]; then
            echo " - Error: S3 Endpoint is required."
          fi
        done

        while [ -z "${s3_region}" ]; do
          echo    # Move to a new line
          read -rp "S3 Region (optional, ex: us-east-1, or leave empty for 'none'): " s3_region
          if [ -z "${s3_region}" ]; then
            s3_region="none"
          fi
        done

        while [ -z "${s3_id}" ]; do
          echo    # Move to a new line
          read -rp "S3 ID (S3 Access ID with read/write access to the bucket): " s3_id
          if [ -z "${s3_id}" ]; then
            echo " - Error: S3 ID is required."
          fi
        done

        while [ -z "${s3_secret}" ]; do
          echo    # Move to a new line
          read -rsp "S3 Secret (S3 Secret Key with read/write access to the bucket): " s3_secret
          echo    # Move to a new line
          if [ -z "${s3_secret}" ]; then
            echo " - Error: S3 Secret is required."
          fi
        done
      }

      # Function to prompt for local disk storage details
      prompt_local_storage() {
        local_path=""

        while [ -z "${local_path}" ]; do
          echo    # Move to a new line
          read -rp "Local Path (Full path to local directory for backups - example: /mnt/backup/storage): " local_path
          if [ -z "${local_path}" ]; then
            echo " - Error: Local Path is required."
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
          read -rp "Storage Type (Currently support local, sftp, b2, and s3): " type
          if [[ "${type}" == "local" ]]; then
            prompt_local_storage
            break
          elif [[ "${type}" == "sftp" ]]; then
            prompt_sftp_storage
            break
          elif [[ "${type}" == "b2" ]]; then
            prompt_b2_storage
            break
          elif [[ "${type}" == "s3" ]]; then
            prompt_s3_storage
            break
          else
            echo "Unsupported storage type. Please enter either 'local', 'sftp', 'b2', or 's3'."
          fi
        done

        # Write storage target details to config file
        # Use printf to handle special characters in user input (like $ in passwords)
        {
          printf 'STORAGE_TARGET_%s_NAME="%s"\n' "${i}" "${name}"
          printf 'STORAGE_TARGET_%s_TYPE="%s"\n' "${i}" "${type}"
        } >> "${ARCHIVER_DIR}/config.sh"

        if [[ $type == "local" ]]; then
          {
            printf 'STORAGE_TARGET_%s_LOCAL_PATH="%s"\n' "${i}" "${local_path}"
            printf '\n'
          } >> "${ARCHIVER_DIR}/config.sh"
        elif [[ $type == "sftp" ]]; then
          {
            printf 'STORAGE_TARGET_%s_SFTP_URL="%s"\n' "${i}" "${sftp_url}"
            printf 'STORAGE_TARGET_%s_SFTP_PORT="%s"\n' "${i}" "${sftp_port}"
            printf 'STORAGE_TARGET_%s_SFTP_USER="%s"\n' "${i}" "${sftp_user}"
            printf 'STORAGE_TARGET_%s_SFTP_PATH="%s"\n' "${i}" "${sftp_path}"
            printf 'STORAGE_TARGET_%s_SFTP_KEY_FILE="%s"\n' "${i}" "${sftp_key_file}"
            printf '\n'
          } >> "${ARCHIVER_DIR}/config.sh"
        elif [[ $type == "b2" ]]; then
          {
            printf 'STORAGE_TARGET_%s_B2_BUCKETNAME="%s"\n' "${i}" "${b2_bucketname}"
            printf 'STORAGE_TARGET_%s_B2_ID="%s"\n' "${i}" "${b2_id}"
            printf 'STORAGE_TARGET_%s_B2_KEY="%s"\n' "${i}" "${b2_key}"
            printf '\n'
          } >> "${ARCHIVER_DIR}/config.sh"
        elif [[ $type == "s3" ]]; then
          {
            printf 'STORAGE_TARGET_%s_S3_BUCKETNAME="%s"\n' "${i}" "${s3_bucketname}"
            printf 'STORAGE_TARGET_%s_S3_ENDPOINT="%s"\n' "${i}" "${s3_endpoint}"
            printf 'STORAGE_TARGET_%s_S3_REGION="%s"\n' "${i}" "${s3_region}"
            printf 'STORAGE_TARGET_%s_S3_ID="%s"\n' "${i}" "${s3_id}"
            printf 'STORAGE_TARGET_%s_S3_SECRET="%s"\n' "${i}" "${s3_secret}"
            printf '\n'
          } >> "${ARCHIVER_DIR}/config.sh"
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
#   using the same X number, as in the below examples. Local, SFTP, BackBlaze B2, and S3 storage
#   targets are currently supported. Require at least one storage target.

# Example Local Disk Storage Target
  # STORAGE_TARGET_1_NAME="name" # You can call this whatever you want, but it must be unique.
  # STORAGE_TARGET_1_TYPE="local" # Currently support local, sftp, b2, and s3. For local, require LOCAL_PATH as below.
  # STORAGE_TARGET_1_LOCAL_PATH="/mnt/backup/storage" # Full path to local directory for backups.

# Example SFTP Storage Target
  # STORAGE_TARGET_2_NAME="name" # You can call this whatever you want, but it must be unique.
  # STORAGE_TARGET_2_TYPE="sftp" # Currently support local, sftp, b2, and s3. For sftp, require URL, PORT, USER, PATH, and KEY_FILE as below.
  # STORAGE_TARGET_2_SFTP_URL="192.168.1.1" # The IP address or FQDN of the sftp host.
  # STORAGE_TARGET_2_SFTP_PORT="22" # The sftp port of the sftp host. Default is 22.
  # STORAGE_TARGET_2_SFTP_USER="user" # User with sftp privileges on sftp host.
  # STORAGE_TARGET_2_SFTP_PATH="remote/path" # Absolute path to remote backup directory. For synology, this starts with the name of the shared folder.
  # STORAGE_TARGET_2_SFTP_KEY_FILE="/path/to/id_ed25519" # Full path to private ssh key file.

# Example B2 Storage Target
  # STORAGE_TARGET_3_NAME="name" # You can call this whatever you want, but it must be unique.
  # STORAGE_TARGET_3_TYPE="b2" # Currently support local, sftp, b2, and s3. For b2, require BUCKETNAME, ID, and KEY as below.
  # STORAGE_TARGET_3_B2_BUCKETNAME="bucketName" # BackBlaze bucket name. Must be globally unique.
  # STORAGE_TARGET_3_B2_ID="keyID" # BackBlaze keyID with read/write access to the above bucket.
  # STORAGE_TARGET_3_B2_KEY="applicationKey" # BackBlaze applicationKey with read/write access to the above bucket.

# Example S3 Storage Target
  # STORAGE_TARGET_4_NAME="name" # You can call this whatever you want, but it must be unique.
  # STORAGE_TARGET_4_TYPE="s3" # Currently support local, sftp, b2, and s3. For s3, require BUCKETNAME, ENDPOINT, ID, and SECRET as below.
  # STORAGE_TARGET_4_S3_BUCKETNAME="bucketName" # S3 bucket name. Must be globally unique.
  # STORAGE_TARGET_4_S3_ENDPOINT="endpoint" # S3 endpoint (ex: amazon.com or hel1.your-objectstorage.com).
  # STORAGE_TARGET_4_S3_REGION="none" # S3 region (optional, depending on service. ex: us-east-1)
  # STORAGE_TARGET_4_S3_ID="id" # S3 Access ID with read/write access to the bucket.
  # STORAGE_TARGET_4_S3_SECRET="secret" # S3 Secret Key with read/write access to the bucket.

EOL

      # Write secrets to config file using printf to handle special characters
      {
        printf '# Secrets for all Duplicacy storage targets\n'
        printf 'STORAGE_PASSWORD="%s" # Password for Duplicacy storage (required)\n' "${storage_password}"
        printf 'RSA_PASSPHRASE="%s" # Passphrase for RSA private key (required)\n' "${RSA_PASSPHRASE}"
        printf '\n\n'
      } >> "${ARCHIVER_DIR}/config.sh"

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

      # Write notification config to file
      # Use printf for API tokens/keys to handle special characters
      {
        printf '# ------------------ #\n'
        printf '# OPTIONAL VARIABLES #\n'
        printf '# ------------------ #\n'
        printf '# Notifications\n'
        printf 'NOTIFICATION_SERVICE="%s" # Currently support '"'"'None'"'"' or '"'"'Pushover'"'"'\n' "${notification_service}"
        printf 'PUSHOVER_USER_KEY="%s" # Pushover user key (not email address), viewable when logged into Pushover dashboard\n' "${pushover_user_key}"
        printf 'PUSHOVER_API_TOKEN="%s" # Pushover application API token/key\n' "${pushover_api_token}"
        printf '\n'
      } >> "${ARCHIVER_DIR}/config.sh"

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

create_new_bundle() {
  # Check if bundle file exists
  if [ ! -f "${BUNDLE_DIR}/bundle.tar.enc" ]; then
    echo    # Move to a new line
    echo "Creating encrypted bundle file..."
    "${BUNDLE_EXPORT_SCRIPT}"
  else
    echo    # Move to a new line
    echo " - Bundle file already exists: ${BUNDLE_DIR}/bundle.tar.enc"
    echo " - Run 'archiver bundle export' to update it (old version will be saved as .old)"
  fi
}


main() {
  import_if_missing

  create_keys_dir

  generate_rsa_keypair

  generate_ssh_keypair

  create_config_file

  create_new_bundle

  sleep 2

  echo    # Move to a new line
  echo    # Move to a new line
  echo " - Init script completed."
  echo "IMPORTANT: You MUST keep a separate backup of your config.sh file and your keys directory."
  echo " - This script has created a password protected bundle file."
  echo " - Keep this bundle file safe - you'll need it to run Archiver in Docker."
}

main
