#!/usr/bin/env bats
# lib/core/config-serialize.sh backs `bundle export` and `archiver migrate`: the effective
# config must round-trip through both output shapes byte-exactly, including passwords with
# shell metacharacters — a quoting bug here corrupts credentials in every exported bundle.

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  SECRETS_DIR="${BATS_TEST_TMPDIR}/secrets"
  CONFIG_FILE="${BATS_TEST_TMPDIR}/config.sh"
  mkdir -p "${SECRETS_DIR}"
  : >"${CONFIG_FILE}"
  export SECRETS_DIR CONFIG_FILE
}

# config-serialize needs config-loader's CONFIG_*_VARS_RE, so load that first (same
# arrange-then-source pattern as layered_config.bats), then the serializer.
load_serializer() {
  COMMON_SH_SOURCED=true
  LOGGING_SH_SOURCED=true
  CONFIG_LOADER_SH_SOURCED=true
  source_if_not_sourced() { :; }
  log_message() { :; }
  handle_error() { echo "handle_error: $*" >&2; return 1; }
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-loader.sh"
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-serialize.sh"
}

arrange_effective_config() {
  SERVICE_DIRECTORIES=(/srv/app /home/user/data)
  STORAGE_TARGET_1_NAME="do-spaces"
  STORAGE_TARGET_1_TYPE="s3"
  STORAGE_TARGET_1_S3_BUCKETNAME="bucket"
  STORAGE_TARGET_1_S3_ENDPOINT="nyc3.digitaloceanspaces.com"
  STORAGE_TARGET_1_S3_REGION="nyc3"
  STORAGE_TARGET_1_S3_ID="AKIAEXAMPLE"
  STORAGE_TARGET_1_S3_SECRET='se$cret "with" spaces & $(dollar)'"'"'quote'
  ROTATE_BACKUPS="true"
  PRUNE_KEEP="-keep 0:180 -keep 30:30"
  STORAGE_PASSWORD='pa$$w0rd with spaces!'
  RSA_PASSPHRASE='phrase"double'
}

@test "serialize_config_sh: nasty secrets survive a source round-trip byte-exactly" {
  load_serializer
  arrange_effective_config
  OUT="${BATS_TEST_TMPDIR}/out-config.sh"
  serialize_config_sh "${OUT}"

  WANT_SECRET="${STORAGE_TARGET_1_S3_SECRET}"
  WANT_PW="${STORAGE_PASSWORD}"
  GOT_SECRET="$(bash -c "source '${OUT}'; printf '%s' \"\${STORAGE_TARGET_1_S3_SECRET}\"")"
  GOT_PW="$(bash -c "source '${OUT}'; printf '%s' \"\${STORAGE_PASSWORD}\"")"
  GOT_KEEP="$(bash -c "source '${OUT}'; printf '%s' \"\${PRUNE_KEEP}\"")"
  [ "${GOT_SECRET}" = "${WANT_SECRET}" ]
  [ "${GOT_PW}" = "${WANT_PW}" ]
  [ "${GOT_KEEP}" = "${PRUNE_KEEP}" ]
}

@test "serialize_config_sh: SERVICE_DIRECTORIES array is emitted as the canonical colon-scalar" {
  load_serializer
  arrange_effective_config
  OUT="${BATS_TEST_TMPDIR}/out-config.sh"
  serialize_config_sh "${OUT}"
  GOT="$(bash -c "source '${OUT}'; printf '%s' \"\${SERVICE_DIRECTORIES}\"")"
  [ "${GOT}" = "/srv/app:/home/user/data" ]
}

@test "serialize_env_and_secrets: secrets land as exact-byte files (mode 600), never in the env file" {
  load_serializer
  arrange_effective_config
  ENVFILE="${BATS_TEST_TMPDIR}/archiver.env"
  OUTSECRETS="${BATS_TEST_TMPDIR}/out-secrets"
  serialize_env_and_secrets "${ENVFILE}" "${OUTSECRETS}"

  [ "$(cat "${OUTSECRETS}/storage_password")" = "${STORAGE_PASSWORD}" ]
  [ "$(cat "${OUTSECRETS}/storage_target_1_s3_secret")" = "${STORAGE_TARGET_1_S3_SECRET}" ]
  [ "$(stat -f '%Lp' "${OUTSECRETS}/storage_password" 2>/dev/null || stat -c '%a' "${OUTSECRETS}/storage_password")" = "600" ]

  grep -q "^SERVICE_DIRECTORIES=/srv/app:/home/user/data$" "${ENVFILE}"
  grep -q "^ROTATE_BACKUPS=true$" "${ENVFILE}"
  ! grep -q "STORAGE_PASSWORD" "${ENVFILE}"
  ! grep -q "S3_SECRET" "${ENVFILE}"
  ! grep -q "S3_ID" "${ENVFILE}"
}

@test "serialize_env_and_secrets: key files are copied under their /run/secrets names (ssh public too)" {
  load_serializer
  arrange_effective_config
  KEYDIR="${BATS_TEST_TMPDIR}/keys"
  mkdir -p "${KEYDIR}"
  DUPLICACY_RSA_PRIVATE_KEY_FILE="${KEYDIR}/private.pem"
  DUPLICACY_RSA_PUBLIC_KEY_FILE="${KEYDIR}/public.pem"
  DUPLICACY_SSH_PRIVATE_KEY_FILE="${KEYDIR}/id_ed25519"
  DUPLICACY_SSH_PUBLIC_KEY_FILE="${KEYDIR}/id_ed25519.pub"
  echo "rsa-private" >"${DUPLICACY_RSA_PRIVATE_KEY_FILE}"
  echo "rsa-public"  >"${DUPLICACY_RSA_PUBLIC_KEY_FILE}"
  echo "ssh-private" >"${DUPLICACY_SSH_PRIVATE_KEY_FILE}"
  echo "ssh-public"  >"${DUPLICACY_SSH_PUBLIC_KEY_FILE}"

  ENVFILE="${BATS_TEST_TMPDIR}/archiver.env"
  OUTSECRETS="${BATS_TEST_TMPDIR}/out-secrets"
  serialize_env_and_secrets "${ENVFILE}" "${OUTSECRETS}"

  [ "$(cat "${OUTSECRETS}/rsa_private_key")" = "rsa-private" ]
  [ "$(cat "${OUTSECRETS}/rsa_public_key")" = "rsa-public" ]
  [ "$(cat "${OUTSECRETS}/ssh_private_key")" = "ssh-private" ]
  # the SFTP restore path needs the public half too
  [ "$(cat "${OUTSECRETS}/ssh_public_key")" = "ssh-public" ]
}
