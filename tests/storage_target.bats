#!/usr/bin/env bats
# The two helpers that de-duplicate storage handling across backup + restore:
#   build_storage_url              - pure URL string per storage type
#   export_duplicacy_storage_secrets - the sanitize->UPPER->DUPLICACY_<NAME>_* credential
#                                      env mapping the do-spaces incidents traced back to
# Both live in lib/core/config-loader.sh and are the single source of truth used by
# duplicacy_primary_backup, duplicacy_add_backup, and duplicacy_init_for_restore.

load 'helpers/load'

setup() {
  load_config_loader
}

# --- build_storage_url -------------------------------------------------------

@test "build_storage_url: local returns the raw path" {
  STORAGE_TARGET_1_TYPE="local"
  STORAGE_TARGET_1_LOCAL_PATH="/mnt/backups/local"
  run build_storage_url 1
  [ "$status" -eq 0 ]
  [ "$output" = "/mnt/backups/local" ]
}

@test "build_storage_url: sftp uses the user@host:port//path shape (double slash)" {
  STORAGE_TARGET_1_TYPE="sftp"
  STORAGE_TARGET_1_SFTP_USER="backup"
  STORAGE_TARGET_1_SFTP_URL="sftp.example.com"
  STORAGE_TARGET_1_SFTP_PORT="2222"
  STORAGE_TARGET_1_SFTP_PATH="srv/duplicacy"
  run build_storage_url 1
  [ "$output" = "sftp://backup@sftp.example.com:2222//srv/duplicacy" ]
}

@test "build_storage_url: b2 uses b2://bucket" {
  STORAGE_TARGET_1_TYPE="b2"
  STORAGE_TARGET_1_B2_BUCKETNAME="my-b2-bucket"
  run build_storage_url 1
  [ "$output" = "b2://my-b2-bucket" ]
}

@test "build_storage_url: s3 embeds the configured region" {
  STORAGE_TARGET_1_TYPE="s3"
  STORAGE_TARGET_1_S3_REGION="us-east-1"
  STORAGE_TARGET_1_S3_ENDPOINT="s3.amazonaws.com"
  STORAGE_TARGET_1_S3_BUCKETNAME="archive"
  run build_storage_url 1
  [ "$output" = "s3://us-east-1@s3.amazonaws.com/archive" ]
}

@test "build_storage_url: s3 with no region defaults to 'none'" {
  STORAGE_TARGET_1_TYPE="s3"
  STORAGE_TARGET_1_S3_REGION=""
  STORAGE_TARGET_1_S3_ENDPOINT="nyc3.digitaloceanspaces.com"
  STORAGE_TARGET_1_S3_BUCKETNAME="archive"
  run build_storage_url 1
  [ "$output" = "s3://none@nyc3.digitaloceanspaces.com/archive" ]
}

@test "build_storage_url: unsupported type returns non-zero and prints nothing" {
  STORAGE_TARGET_1_TYPE="ftp"
  run build_storage_url 1
  [ "$status" -ne 0 ]
  [ -z "$output" ]
}

@test "build_storage_url: reads the target-specific id (copy target #2)" {
  STORAGE_TARGET_1_TYPE="local"
  STORAGE_TARGET_1_LOCAL_PATH="/primary"
  STORAGE_TARGET_2_TYPE="b2"
  STORAGE_TARGET_2_B2_BUCKETNAME="secondary-bucket"
  run build_storage_url 2
  [ "$output" = "b2://secondary-bucket" ]
}

# --- export_duplicacy_storage_secrets ---------------------------------------
# Called directly (not via `run`): `run` executes in a subshell, so its exports
# would vanish before we could assert them.

@test "export_duplicacy_storage_secrets: local exports only the password" {
  STORAGE_TARGET_1_NAME="localdisk"
  STORAGE_TARGET_1_TYPE="local"
  STORAGE_PASSWORD="s3cret"

  export_duplicacy_storage_secrets 1

  [ "${DUPLICACY_LOCALDISK_PASSWORD}" = "s3cret" ]
  [ -z "${DUPLICACY_LOCALDISK_S3_ID:-}" ]
  [ -z "${DUPLICACY_LOCALDISK_B2_ID:-}" ]
}

@test "export_duplicacy_storage_secrets: hyphenated s3 name maps to DUPLICACY_DO_SPACES_* (do-spaces incident lock)" {
  STORAGE_TARGET_1_NAME="do-spaces"
  STORAGE_TARGET_1_TYPE="s3"
  STORAGE_PASSWORD="pw"
  STORAGE_TARGET_1_S3_ID="AKIAxxx"
  STORAGE_TARGET_1_S3_SECRET="shhh"

  export_duplicacy_storage_secrets 1

  # The hyphen must become an underscore and the name upper-cased, or duplicacy
  # never sees these creds (the 0.8.10 regression).
  [ "${DUPLICACY_DO_SPACES_PASSWORD}" = "pw" ]
  [ "${DUPLICACY_DO_SPACES_S3_ID}" = "AKIAxxx" ]
  [ "${DUPLICACY_DO_SPACES_S3_SECRET}" = "shhh" ]
}

@test "export_duplicacy_storage_secrets: b2 exports id + key for the target" {
  STORAGE_TARGET_2_NAME="b2copy"
  STORAGE_TARGET_2_TYPE="b2"
  STORAGE_PASSWORD="pw"
  STORAGE_TARGET_2_B2_ID="b2id"
  STORAGE_TARGET_2_B2_KEY="b2key"

  export_duplicacy_storage_secrets 2

  [ "${DUPLICACY_B2COPY_PASSWORD}" = "pw" ]
  [ "${DUPLICACY_B2COPY_B2_ID}" = "b2id" ]
  [ "${DUPLICACY_B2COPY_B2_KEY}" = "b2key" ]
}

@test "export_duplicacy_storage_secrets: sftp exports the private key file path" {
  STORAGE_TARGET_1_NAME="sftpstore"
  STORAGE_TARGET_1_TYPE="sftp"
  STORAGE_PASSWORD="pw"
  DUPLICACY_SSH_PRIVATE_KEY_FILE="/opt/archiver/keys/id_ed25519"

  export_duplicacy_storage_secrets 1

  [ "${DUPLICACY_SFTPSTORE_PASSWORD}" = "pw" ]
  [ "${DUPLICACY_SFTPSTORE_SSH_KEY_FILE}" = "/opt/archiver/keys/id_ed25519" ]
}
