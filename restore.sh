#!/bin/bash

handle_error() {
  echo "Error: $1"
  exit 1
}

# Check if expect is installed
if ! command -v expect &> /dev/null; then
    handle_error "Expect is not installed. Please install it using 'sudo apt install expect'."
fi

read -r -p 'Service Name (default: adguard): ' SERVICE
SERVICE=${SERVICE:-adguard}
echo "Service Name: $SERVICE"

read -r -p 'Parent directory (default: /srv): ' PARENT
PARENT=${PARENT:-/srv}
echo "Parent directory: $PARENT"

if [[ "${SERVICE}" =~ ^(audiobookshelf|frigate|immich|media-server|nextcloud|paperless)$ ]]; then

  read -r -p 'Credentials File Location (default: /home/cody/.smbcredentials): ' CREDENTIALS_FILE
  CREDENTIALS_FILE=${CREDENTIALS_FILE:-/home/cody/.smbcredentials}
  echo "Credentials File Location: $CREDENTIALS_FILE"

  if [ ! -f "$CREDENTIALS_FILE" ]; then

    read -r -p 'NAS Username (default: server): ' NAS_USERNAME
    NAS_USERNAME=${NAS_USERNAME:-server}
    echo "NAS Username: $NAS_USERNAME"

    read -r -s -p 'NAS Password (required): ' NAS_PASSWORD
    echo
    if [ -z "$NAS_PASSWORD" ]; then
      echo "Error: NAS Password is required."
      exit 1
    fi
    echo "NAS Password: [hidden]"

    echo "Creating credentials file..."

    # Create the file with the given username and password
    sudo bash -c "echo -e 'username=${NAS_USERNAME}\npassword=${NAS_PASSWORD}' > $CREDENTIALS_FILE" || handle_error "Failed to create credentials file."

    # Set the owner and permissions
    sudo chown 1000:1000 "$CREDENTIALS_FILE" || handle_error "Failed to set ownership for credentials file."
    sudo chmod 0600 "$CREDENTIALS_FILE" || handle_error "Failed to set permissions for credentials file."

    echo "Credentials file created successfully."
  else
    echo "Credentials file already exists."
  fi

  read -r -p 'NAS URL (default: synology.internal): ' NAS_URL
  NAS_URL=${NAS_URL:-synology.internal}
  echo "NAS URL: $NAS_URL"

  MOUNT_POINT="${PARENT}/${SERVICE}/volumes/mnt/${SERVICE}-assets"

  mkdir -p "${MOUNT_POINT}" || handle_error "Failed to create mnt directory."

  FSTAB_LINE="//${NAS_URL}/${SERVICE}-assets ${MOUNT_POINT} cifs credentials=${CREDENTIALS_FILE},_netdev,file_mode=0777,dir_mode=0777,uid=1000,gid=1000 0 0"

  # Check if the mount point is already mounted
  if ! mountpoint -q "$MOUNT_POINT"; then

    # Check if the line is already in /etc/fstab
    if ! grep -qF "$FSTAB_LINE" /etc/fstab; then

      # Append the line to /etc/fstab safely
      echo "$FSTAB_LINE" | sudo tee -a /etc/fstab > /dev/null || handle_error "Failed to add line to /etc/fstab."

      # Reload systemd to apply changes
      sudo systemctl daemon-reload || handle_error "Failed to reload systemctl daemon."

    else

      echo "The line is already in /etc/fstab."

    fi

    # Mount the filesystem
    sudo mount "$MOUNT_POINT" || handle_error "Failed to mount $MOUNT_POINT."

  else

    echo "$MOUNT_POINT is already mounted."

  fi

fi

read -r -p 'Storage Name (default: omv): ' STORAGE
STORAGE=${STORAGE:-omv}
echo "Storage Name: $STORAGE"

read -r -p 'Storage URL (default: sftp://server@synology.internal/duplicacy): ' STORAGE_URL
STORAGE_URL=${STORAGE_URL:-sftp://server@synology.internal/duplicacy}
echo "Storage URL: $STORAGE_URL"

read -r -p 'Host Name (default: ubuntu-server): ' HOST
HOST=${HOST:-ubuntu-server}
echo "Host Name: $HOST"

read -r -p 'Public RSA Key File (default: /home/cody/archiver/.keys/public.pem): ' RSA_PUB
RSA_PUB=${RSA_PUB:-/home/cody/archiver/.keys/public.pem}
echo "Public RSA Key File: $RSA_PUB"

read -r -p 'Private RSA Key File (default: /home/cody/archiver/.keys/private.pem): ' RSA_PRIV
RSA_PRIV=${RSA_PRIV:-/home/cody/archiver/.keys/private.pem}
echo "Private RSA Key File: $RSA_PRIV"

read -r -p 'Private SSH Key File (default: /home/cody/archiver/.keys/id_rsa): ' SSH_PRIV
SSH_PRIV=${SSH_PRIV:-/home/cody/archiver/.keys/id_rsa}
echo "Private SSH Key File: $SSH_PRIV"

read -r -s -p 'RSA Key Passphrase (required): ' RSA_PASSPHRASE
echo
if [ -z "$RSA_PASSPHRASE" ]; then
  echo "Error: RSA Key Passphrase is required."
  exit 1
fi
echo "RSA Key Passphrase: [hidden]"

read -r -s -p 'Storage Password (required): ' STORAGE_PASSWORD
echo
if [ -z "$STORAGE_PASSWORD" ]; then
  echo "Error: Storage Password is required."
  exit 1
fi
echo "Storage Password: [hidden]"

# Create Local Directory
mkdir -p -m 0755 "${PARENT}/${SERVICE}"
cd "${PARENT}/${SERVICE}" || handle_error "Failed to change directory to ${PARENT}/${SERVICE}."

# Initializing Duplicacy Repository

# Creating an expect script to handle the prompts
expect <<EOF
spawn sudo duplicacy init -e -key "${RSA_PUB}" -storage-name "${STORAGE}" "${HOST}-${SERVICE}" "${STORAGE_URL}"
expect "Enter the path of the private key file:"
send "${SSH_PRIV}\r"
expect "Enter storage password for ${STORAGE_URL}:"
send "${STORAGE_PASSWORD}\r"
expect eof
EOF

echo "Duplicacy initialization completed successfully."

# Setting duplicacy secrets
sudo duplicacy set -key password -value "${STORAGE_PASSWORD}"
sudo duplicacy set -storage "${STORAGE}" -key rsa_passphrase -value "${RSA_PASSPHRASE}"
sudo duplicacy set -key ssh_key_file -value "${SSH_PRIV}"

sudo duplicacy list #this should give you the info for revision number, needed below

read -r -p 'Choose a revision to restore (default: 1): ' REVISION
REVISION=${REVISION:-1}
echo "Chosen revision: $REVISION"

# Pulling down chosen revision
sudo duplicacy restore -r "${REVISION}" -key "${RSA_PRIV}"

echo "Repository restored."

if [ -f restore.sh ]; then
  echo "Now running restore.sh..."
  bash restore-service.sh
fi
