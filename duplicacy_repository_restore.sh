#/bin/bash

# Check if expect is installed
if ! command -v expect &> /dev/null; then
    echo "Error: expect is not installed. Please install it using 'sudo apt install expect'."
    exit 1
fi

read -r -p 'Service Name (default: adguard): ' SERVICE
SERVICE=${SERVICE:-adguard}
echo "Service Name: $SERVICE"

read -r -p 'Parent directory (default: /srv/): ' PARENT
PARENT=${PARENT:-/srv/}
echo "Parent directory: $PARENT"

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
cd "${PARENT}/${SERVICE}"

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
sudo duplicacy restore -r "${REVISION}" -key $RSA_PRIV

echo "Repository restored. Now running restore.sh..."

bash restore.sh
