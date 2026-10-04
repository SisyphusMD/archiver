#!/bin/bash
# Interactive setup: generates the keys and writes env-native materials (an env file plus
# one file per secret) from the answers given.

INIT_SH_SOURCED=true

set -e

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${REQUIRE_CONTAINER_CORE}"
source_if_not_sourced "${CONFIG_SERIALIZE_CORE}"

# The answers are set as shell variables and serialized from there, so a setting inherited
# from the container's environment (e.g. 'docker compose run ... init') must not ride along.
while IFS= read -r v; do
  unset "${v}"
done < <(compgen -v | grep -E "${CONFIG_NONSECRET_VARS_RE}|${CONFIG_SECRET_VARS_RE}")
unset v

# set_setting NAME VALUE records one answer.
set_setting() { printf -v "${1}" '%s' "${2}"; }

# Generated credentials
GENERATED_RSA_PASSPHRASE=""
GENERATED_STORAGE_PASSWORD=""
GENERATED_RECOVERY_PASSWORD=""

# Helper functions
print_header() {
  echo
  echo "==========================================="
  echo "$1"
  echo "==========================================="
  echo
}

print_section() {
  echo
  echo "-------------------------------------------"
  echo "$1"
  echo "-------------------------------------------"
  echo
}

print_success() {
  echo "✓ $1"
}

print_info() {
  echo "ℹ $1"
}

sanitize_storage_name() {
  # Only allow letters, numbers, and underscores
  # Replace hyphens with underscores, remove other special chars
  echo "$1" | tr -d '[:space:]' | tr '-' '_' | sed 's/[^a-zA-Z0-9_]//g'
}

sanitize_path() {
  # Remove leading/trailing whitespace and validate path structure
  local path="$1"
  path="$(echo "$path" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"

  # Check if path is empty after trimming
  if [ -z "$path" ]; then
    return 1
  fi

  # Check for invalid characters (null bytes, control characters except newline)
  if printf '%s' "$path" | LC_ALL=C grep -q '[[:cntrl:]]'; then
    return 1
  fi

  # For absolute paths, must start with /
  # For relative paths or wildcards, allow them through
  # This is permissive to allow container paths and wildcard patterns like /srv/*
  echo "$path"
}

sanitize_url() {
  # Remove leading/trailing whitespace and trailing slashes, validate URL/hostname
  local url="$1"
  url="$(echo "$url" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | sed 's:/*$::')"

  # Check if empty after trimming
  if [ -z "$url" ]; then
    return 1
  fi

  # Check for invalid characters (spaces, control characters, etc.)
  if printf '%s' "$url" | LC_ALL=C grep -q '[[:cntrl:][:space:]]'; then
    return 1
  fi

  # Basic validation: should contain valid hostname characters
  # Allow: letters, numbers, dots, hyphens (hostnames) and colons (IPv6/ports)
  if ! echo "$url" | grep -qE '^[a-zA-Z0-9]([a-zA-Z0-9._:-]*[a-zA-Z0-9])?$'; then
    return 1
  fi

  echo "$url"
}

validate_port() {
  # Check if port is a valid number between 1-65535
  local port="$1"
  if [[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
    echo "$port"
  else
    echo "22"  # Default to 22 if invalid
  fi
}

generate_password() {
  # Generate a 32-character alphanumeric password
  tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 32
}

backup_existing_file() {
  local file_path="$1"
  if [ -f "$file_path" ]; then
    local backup_path="${file_path}.backup.$(date +%Y%m%d-%H%M%S)"
    mv "$file_path" "$backup_path"
    print_info "Backed up existing file to: $(basename "$backup_path")"
  fi
}


create_keys_dir() {
  if [[ ! -d "${KEYS_DIR}" ]]; then
    mkdir -p "${KEYS_DIR}"
    chmod 700 "${KEYS_DIR}"
  fi
}

generate_rsa_keypair() {
  print_section "Generating RSA Encryption Keys"

  backup_existing_file "${KEYS_DIR}/private.pem"
  backup_existing_file "${KEYS_DIR}/public.pem"

  # Auto-generate passphrase
  GENERATED_RSA_PASSPHRASE=$(generate_password)

  expect <<EOF
spawn openssl genrsa -aes256 -out "${KEYS_DIR}/private.pem" -traditional 2048
expect "Enter PEM pass phrase:"
send "${GENERATED_RSA_PASSPHRASE}\r"
expect "Verifying - Enter PEM pass phrase:"
send "${GENERATED_RSA_PASSPHRASE}\r"
expect eof
EOF

  expect <<EOF
spawn openssl rsa -in "${KEYS_DIR}/private.pem" --outform PEM -pubout -out "${KEYS_DIR}/public.pem"
expect "Enter pass phrase for ${KEYS_DIR}/private.pem:"
send "${GENERATED_RSA_PASSPHRASE}\r"
expect eof
EOF

  chmod 700 "${KEYS_DIR}"
  chmod 600 "${KEYS_DIR}/private.pem"
  chmod 644 "${KEYS_DIR}/public.pem"

  if [ ! -f "${KEYS_DIR}/private.pem" ] || [ ! -f "${KEYS_DIR}/public.pem" ]; then
    echo "Error: RSA key pair generation failed"
    exit 1
  fi

  print_success "RSA key pair generated"
}

generate_ssh_keypair() {
  print_section "Generating SSH Keys for SFTP"

  backup_existing_file "${KEYS_DIR}/id_ed25519"
  backup_existing_file "${KEYS_DIR}/id_ed25519.pub"

  ssh-keygen -t ed25519 -f "${KEYS_DIR}/id_ed25519" -N "" -C "archiver" >/dev/null 2>&1

  chmod 700 "${KEYS_DIR}"
  chmod 600 "${KEYS_DIR}/id_ed25519"
  chmod 644 "${KEYS_DIR}/id_ed25519.pub"

  if [ ! -f "${KEYS_DIR}/id_ed25519" ] || [ ! -f "${KEYS_DIR}/id_ed25519.pub" ]; then
    echo "Error: SSH key pair generation failed"
    exit 1
  fi

  print_success "SSH key pair generated"
}

prompt_service_directories() {
  local service_directories_input=""

  echo "Specify directories to backup (comma-separated):"
  echo "  - Use full paths (e.g., /srv/*, /home/user)"
  echo "  - Use * for all subdirectories (e.g., /srv/*/ backs up each subdir separately)"
  echo

  while [ -z "${service_directories_input}" ]; do
    read -p "Directories: " -r service_directories_input
    if [ -z "${service_directories_input}" ]; then
      echo "Error: At least one directory is required"
    fi
  done

  # Split by comma and sanitize each path
  IFS=',' read -r -a raw_dirs <<< "$service_directories_input"
  SERVICE_DIRECTORIES=()
  for dir in "${raw_dirs[@]}"; do
    local sanitized_dir="$(sanitize_path "$dir")"
    if [ -n "$sanitized_dir" ]; then
      SERVICE_DIRECTORIES+=("$sanitized_dir")
    fi
  done
}

prompt_storage_target() {
  local storage_num=$1
  local is_primary=$2

  if [ "$is_primary" = "true" ]; then
    echo "Configure primary storage (required):"
  else
    echo "Configure storage target #${storage_num}:"
  fi
  echo

  local name=""
  while [ -z "${name}" ]; do
    read -p "  Storage name: " -r name
    name="$(sanitize_storage_name "$name")"
    if [ -z "${name}" ]; then
      echo "  Error: Storage name is required (letters, numbers, underscores only)"
    fi
  done

  local type=""
  while [[ ! "$type" =~ ^(local|sftp|b2|s3)$ ]]; do
    read -p "  Storage type (local/sftp/b2/s3): " -r type
    type="$(echo "$type" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')"
    if [[ ! "$type" =~ ^(local|sftp|b2|s3)$ ]]; then
      echo "  Error: Must be local, sftp, b2, or s3"
    fi
  done

    set_setting "STORAGE_TARGET_${storage_num}_NAME" "${name}"
    set_setting "STORAGE_TARGET_${storage_num}_TYPE" "${type}"

  # Type-specific prompts
  case "$type" in
    local)
      local local_path=""
      while [ -z "${local_path}" ]; do
        read -p "  Local path: " -r local_path
        local_path="$(sanitize_path "$local_path")"
        if [ -z "${local_path}" ]; then
          echo "  Error: Path cannot be empty"
        fi
      done
      set_setting "STORAGE_TARGET_${storage_num}_LOCAL_PATH" "${local_path}"
      ;;

    sftp)
      local sftp_url sftp_port sftp_user sftp_path
      while [ -z "${sftp_url}" ]; do
        read -p "  SFTP host (IP or FQDN): " -r sftp_url
        sftp_url="$(sanitize_url "$sftp_url")"
        if [ -z "${sftp_url}" ]; then
          echo "  Error: Host cannot be empty"
        fi
      done
      read -p "  SFTP port [22]: " -r sftp_port
      sftp_port="$(validate_port "${sftp_port:-22}")"
      while [ -z "${sftp_user}" ]; do
        read -p "  SFTP user: " -r sftp_user
        sftp_user="$(echo "$sftp_user" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${sftp_user}" ]; then
          echo "  Error: User cannot be empty"
        fi
      done
      while [ -z "${sftp_path}" ]; do
        read -p "  SFTP path: " -r sftp_path
        sftp_path="$(sanitize_path "$sftp_path")"
        # Remove leading and trailing slashes for SFTP paths
        sftp_path="$(echo "${sftp_path}" | sed 's|^/*||;s|/*$||')"
        if [ -z "${sftp_path}" ]; then
          echo "  Error: Path cannot be empty"
        fi
      done

        set_setting "STORAGE_TARGET_${storage_num}_SFTP_URL" "${sftp_url}"
        set_setting "STORAGE_TARGET_${storage_num}_SFTP_PORT" "${sftp_port}"
        set_setting "STORAGE_TARGET_${storage_num}_SFTP_USER" "${sftp_user}"
        set_setting "STORAGE_TARGET_${storage_num}_SFTP_PATH" "${sftp_path}"
      ;;

    b2)
      local b2_bucket b2_id b2_key
      while [ -z "${b2_bucket}" ]; do
        read -p "  B2 bucket name: " -r b2_bucket
        b2_bucket="$(echo "$b2_bucket" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${b2_bucket}" ]; then
          echo "  Error: Bucket name cannot be empty"
        fi
      done
      while [ -z "${b2_id}" ]; do
        read -p "  B2 key ID: " -r b2_id
        b2_id="$(echo "$b2_id" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${b2_id}" ]; then
          echo "  Error: Key ID cannot be empty"
        fi
      done
      while [ -z "${b2_key}" ]; do
        read -rsp "  B2 application key: " b2_key
        echo
        b2_key="$(echo "$b2_key" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${b2_key}" ]; then
          echo "  Error: Application key cannot be empty"
        fi
      done

        set_setting "STORAGE_TARGET_${storage_num}_B2_BUCKETNAME" "${b2_bucket}"
        set_setting "STORAGE_TARGET_${storage_num}_B2_ID" "${b2_id}"
        set_setting "STORAGE_TARGET_${storage_num}_B2_KEY" "${b2_key}"
      ;;

    s3)
      local s3_bucket s3_endpoint s3_region s3_id s3_secret
      while [ -z "${s3_bucket}" ]; do
        read -p "  S3 bucket name: " -r s3_bucket
        s3_bucket="$(echo "$s3_bucket" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${s3_bucket}" ]; then
          echo "  Error: Bucket name cannot be empty"
        fi
      done
      while [ -z "${s3_endpoint}" ]; do
        read -p "  S3 endpoint: " -r s3_endpoint
        s3_endpoint="$(sanitize_url "$s3_endpoint")"
        if [ -z "${s3_endpoint}" ]; then
          echo "  Error: Endpoint cannot be empty"
        fi
      done
      read -p "  S3 region [none]: " -r s3_region
      s3_region="$(echo "${s3_region:-none}" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
      while [ -z "${s3_id}" ]; do
        read -p "  S3 access key ID: " -r s3_id
        s3_id="$(echo "$s3_id" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${s3_id}" ]; then
          echo "  Error: Access key ID cannot be empty"
        fi
      done
      while [ -z "${s3_secret}" ]; do
        read -rsp "  S3 secret key: " s3_secret
        echo
        s3_secret="$(echo "$s3_secret" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        if [ -z "${s3_secret}" ]; then
          echo "  Error: Secret key cannot be empty"
        fi
      done

        set_setting "STORAGE_TARGET_${storage_num}_S3_BUCKETNAME" "${s3_bucket}"
        set_setting "STORAGE_TARGET_${storage_num}_S3_ENDPOINT" "${s3_endpoint}"
        set_setting "STORAGE_TARGET_${storage_num}_S3_REGION" "${s3_region}"
        set_setting "STORAGE_TARGET_${storage_num}_S3_ID" "${s3_id}"
        set_setting "STORAGE_TARGET_${storage_num}_S3_SECRET" "${s3_secret}"
      ;;
  esac

  print_success "Storage target configured"
}

create_config_file() {
  print_header "Configuration Setup"

  # Auto-generate the storage password and the recovery-kit password. The recovery
  # password must differ from the storage password (the kit contains it); with two
  # 32-char random values a collision is theoretical, but the guard costs nothing.
  GENERATED_STORAGE_PASSWORD=$(generate_password)
  GENERATED_RECOVERY_PASSWORD=$(generate_password)
  while [ "${GENERATED_RECOVERY_PASSWORD}" = "${GENERATED_STORAGE_PASSWORD}" ]; do
    GENERATED_RECOVERY_PASSWORD=$(generate_password)
  done

  # Service directories (prompt_service_directories sets the SERVICE_DIRECTORIES array)
  prompt_service_directories

  set_setting STORAGE_PASSWORD "${GENERATED_STORAGE_PASSWORD}"
  set_setting RSA_PASSPHRASE "${GENERATED_RSA_PASSPHRASE}"
  set_setting RECOVERY_PASSWORD "${GENERATED_RECOVERY_PASSWORD}"

  # Storage targets
  print_section "Storage Configuration"
  prompt_storage_target 1 true

  # Additional storage targets
  local storage_num=2
  while true; do
    echo
    read -p "Add another storage target? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      break
    fi
    echo
    prompt_storage_target "$storage_num" false
    storage_num=$((storage_num + 1))
  done

  # Optional: Pushover
  echo
  read -p "Setup Pushover notifications? (y/N): " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    local pushover_user pushover_token
    echo
    read -p "  Pushover user key: " -r pushover_user
    pushover_user="$(echo "$pushover_user" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    read -p "  Pushover API token: " -r pushover_token
    pushover_token="$(echo "$pushover_token" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    set_setting NOTIFICATION_SERVICE "Pushover"
    set_setting PUSHOVER_USER_KEY "${pushover_user}"
    set_setting PUSHOVER_API_TOKEN "${pushover_token}"
  fi

  # Defaults (maintenance runs on MAINTENANCE_SCHEDULE or 'archiver maintenance')
  set_setting CHECK_BACKUPS "true"
  set_setting PRUNE_BACKUPS "true"
  set_setting PRUNE_KEEP "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
  set_setting PRUNE_EXHAUSTIVE_FREQUENCY "monthly"
  set_setting DUPLICACY_THREADS "4"

  print_success "Configuration recorded"
}

# Env-native materials (env vars + file secrets) are how Archiver is configured; write them
# so a new user never hand-transcribes the configuration.
emit_env_native_materials() {
  print_section "Writing Env-Native Deployment Materials"

  local out="${SETUP_DIR}/env-native"
  (umask 077 && mkdir -p "${out}" && serialize_env_and_secrets "${out}/archiver.env" "${out}/secrets")
  print_success "Env-native materials written to env-native/ in the mounted setup directory"
}

display_credentials() {
  print_header "Setup Complete!"

  echo "IMPORTANT: Save this password in your password manager NOW. Once your deployment"
  echo "is running, Archiver keeps an encrypted recovery kit (every setting, secret, and"
  echo "key) on EVERY storage target, refreshed automatically when anything changes. This"
  echo "one password plus any one reachable storage location recovers everything, on any"
  echo "machine, with no other files needed."
  echo
  echo "┌─────────────────────────────────────────────────────────────┐"
  echo "│ RECOVERY PASSWORD: the single key to recover everything    │"
  echo "├─────────────────────────────────────────────────────────────┤"
  echo "│                                                             │"
  printf "│   %-57s │\n" "${GENERATED_RECOVERY_PASSWORD}"
  echo "│                                                             │"
  echo "└─────────────────────────────────────────────────────────────┘"
  echo
  if [ -f "${KEYS_DIR}/id_ed25519.pub" ]; then
    echo "SSH Public Key (for SFTP servers):"
    echo "────────────────────────────────────────────────────────────"
    cat "${KEYS_DIR}/id_ed25519.pub"
    echo "────────────────────────────────────────────────────────────"
    echo
    echo "Copy this key to your SFTP server's authorized_keys file."
    echo
  fi

  echo "Next steps:"
  echo "  1. Load env-native/archiver.env as environment variables and the"
  echo "     env-native/secrets/ files as secrets mounted under /run/secrets (see the compose template),"
  echo "     then DELETE env-native/ from the setup directory: it holds your secrets in plaintext."
  echo "  2. Start container: docker compose up -d"
  echo
}

main() {
  print_header "Archiver Initialization"

  create_keys_dir
  generate_rsa_keypair
  generate_ssh_keypair
  create_config_file
  emit_env_native_materials
  display_credentials
}

main
