# Define primary Duplicacy-related configuration variables.
DUPLICACY_BIN="/usr/local/bin/duplicacy" # Path to Duplicacy binary
DUPLICACY_KEY_DIR="${ARCHIVER_DIR}/.keys" # Path to Duplicacy key directory
DUPLICACY_SSH_KEY_FILE="${DUPLICACY_KEY_DIR}/id_rsa" # SSH key file
DUPLICACY_RSA_PUBLIC_KEY_FILE="${DUPLICACY_KEY_DIR}/public.pem" # Path to RSA public key file for Duplicacy
DUPLICACY_RSA_PRIVATE_KEY_FILE="${DUPLICACY_KEY_DIR}/private.pem" # Path to RSA private key file for Duplicacy

# OMV Duplicacy variables
DUPLICACY_OMV_STORAGE_NAME="omv" # Name of onsite storage for Duplicacy omv storage
DUPLICACY_OMV_STORAGE_URL="${OMV_URL}" # URL for onsite storage for Duplicacy omv storage
DUPLICACY_OMV_SSH_KEY_FILE="${DUPLICACY_SSH_KEY_FILE}" # SSH key file for Duplicacy omv storage
DUPLICACY_OMV_PASSWORD="${STORAGE_PASSWORD}" # Password for Duplicacy omv storage
DUPLICACY_OMV_RSA_PASSPHRASE="${RSA_PASSPHRASE}" # Passphrase for Duplicacy omv storage

# B2 Duplicacy varibles
DUPLICACY_BACKBLAZE_STORAGE_NAME="backblaze" # Name of offsite storage for Duplicacy backblaze storage
DUPLICACY_BACKBLAZE_STORAGE_URL="${B2_URL}" # URL for offsite storage for Duplicacy backblaze storage
DUPLICACY_BACKBLAZE_B2_ID="${B2_ID}" # Key ID for offsite storage for Duplicacy backblaze storage
DUPLICACY_BACKBLAZE_B2_KEY="${B2_KEY}" # Application Key for offsite storage for Duplicacy backblaze storage
DUPLICACY_BACKBLAZE_PASSWORD="${STORAGE_PASSWORD}" # Password for Duplicacy backblaze storage
DUPLICACY_BACKBLAZE_RSA_PASSPHRASE="${RSA_PASSPHRASE}" # Passphrase for Duplicacy backblaze storage
