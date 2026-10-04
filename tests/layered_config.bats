#!/usr/bin/env bats
# Config load in lib/core/config-loader.sh: non-secret settings from env vars, secrets from
# files only.
#
# Unlike config_loader.bats, these tests must control the environment + fixtures BEFORE the
# load-time orchestration runs, so they arrange state and then source config-loader via
# run_load (rather than the shared setup() sourcing it once).

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  SECRETS_DIR="${BATS_TEST_TMPDIR}/secrets"
  mkdir -p "${SECRETS_DIR}"
  export SECRETS_DIR
}

# Arrange-then-source: satisfies the same source guards/stubs as helpers/load.bash, then
# sources config-loader so its top-level layered load runs against the arranged state.
run_load() {
  COMMON_SH_SOURCED=true
  LOGGING_SH_SOURCED=true
  source_if_not_sourced() { :; }
  log_message() { :; }
  handle_error() { echo "handle_error: $*" >&2; return 1; }
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-vars.sh"
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-loader.sh"
}

@test "resolve_secret: reads a secret from <NAME>_FILE" {
  printf 'from-file-var' >"${BATS_TEST_TMPDIR}/pw"
  export STORAGE_PASSWORD_FILE="${BATS_TEST_TMPDIR}/pw"
  run_load
  [ "${STORAGE_PASSWORD}" = "from-file-var" ]
}

@test "resolve_secret: reads a secret from the default \${SECRETS_DIR}/<name>" {
  printf 'from-default-dir' >"${SECRETS_DIR}/storage_password"
  run_load
  [ "${STORAGE_PASSWORD}" = "from-default-dir" ]
}

@test "resolve_secret: trims the trailing newline a file may carry" {
  printf 'trimmed\n' >"${SECRETS_DIR}/rsa_passphrase"
  run_load
  [ "${RSA_PASSPHRASE}" = "trimmed" ]
}

@test "a secret passed as a raw env var is purged (never trusted)" {
  export STORAGE_PASSWORD="raw-env-secret"
  run_load
  [ -z "${STORAGE_PASSWORD}" ]
}

@test "deprecated ROTATE_BACKUPS translates to PRUNE_BACKUPS" {
  export ROTATE_BACKUPS="false"
  run_load
  [ "${PRUNE_BACKUPS}" = "false" ]
  [ -z "${ROTATE_BACKUPS:-}" ]
}

@test "config comes entirely from env + secret files" {
  export STORAGE_TARGET_1_NAME="b2t" STORAGE_TARGET_1_TYPE="b2"
  printf 'the-key' >"${SECRETS_DIR}/storage_target_1_b2_key"
  run_load
  [ "${STORAGE_TARGET_1_NAME}" = "b2t" ]
  [ "${STORAGE_TARGET_1_B2_KEY}" = "the-key" ]
}

@test "normalize_service_directories: colon-delimited scalar becomes an array" {
  export SERVICE_DIRECTORIES="/mnt/a/:/mnt/b/:/srv/*/"
  run_load
  [ "${#SERVICE_DIRECTORIES[@]}" -eq 3 ]
  [ "${SERVICE_DIRECTORIES[0]}" = "/mnt/a/" ]
  [ "${SERVICE_DIRECTORIES[2]}" = "/srv/*/" ]
}

@test "normalize_service_directories: newlines are accepted as separators too" {
  export SERVICE_DIRECTORIES=$'/mnt/a/\n/mnt/b/\n'
  run_load
  [ "${#SERVICE_DIRECTORIES[@]}" -eq 2 ]
  [ "${SERVICE_DIRECTORIES[1]}" = "/mnt/b/" ]
}

@test "purging a raw env secret logs a warning naming the file path to use" {
  export STORAGE_PASSWORD="sneaky-env-secret"
  COMMON_SH_SOURCED=true
  LOGGING_SH_SOURCED=true
  source_if_not_sourced() { :; }
  WARNINGS=""
  log_message() { [ "${1}" = "WARNING" ] && WARNINGS+="${2}"$'\n'; }
  handle_error() { echo "handle_error: $*" >&2; return 1; }
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-vars.sh"
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-loader.sh"
  [ -z "${STORAGE_PASSWORD:-}" ]
  [[ "${WARNINGS}" == *"Ignoring STORAGE_PASSWORD"* ]]
  [[ "${WARNINGS}" == *"storage_password"* ]]
}

@test "resolve_secret strips a trailing CRLF (Windows-edited secret file)" {
  printf 'pw-value\r\n' >"${SECRETS_DIR}/storage_password"
  run_load
  [ "${STORAGE_PASSWORD}" = "pw-value" ]
}

@test "an explicitly set <NAME>_FILE pointing at a missing file is a hard error" {
  export STORAGE_PASSWORD_FILE="${BATS_TEST_TMPDIR}/does-not-exist"
  run run_load
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"STORAGE_PASSWORD_FILE"* ]]
}


@test "a config.sh is never read" {
  CONFIG_FILE="${BATS_TEST_TMPDIR}/config.sh"
  echo 'STORAGE_TARGET_1_NAME="from-config-sh"' >"${CONFIG_FILE}"
  export CONFIG_FILE
  run_load
  [ -z "${STORAGE_TARGET_1_NAME:-}" ]
}

@test "break-glass credentials are read per storage type from secret files" {
  export STORAGE_TARGET_1_NAME="b" STORAGE_TARGET_1_TYPE="b2" STORAGE_TARGET_2_NAME="s" STORAGE_TARGET_2_TYPE="sftp"
  printf 'ro-id' >"${SECRETS_DIR}/storage_target_1_breakglass_b2_id"
  printf 'ro-key' >"${SECRETS_DIR}/storage_target_1_breakglass_b2_key"
  printf -- '-----BEGIN KEY-----\nline2\n-----END KEY-----\n' >"${SECRETS_DIR}/storage_target_2_breakglass_ssh_key"
  run_load
  [ "${STORAGE_TARGET_1_BREAKGLASS_B2_ID}" = "ro-id" ]
  [ "${STORAGE_TARGET_1_BREAKGLASS_B2_KEY}" = "ro-key" ]
  [ "${STORAGE_TARGET_2_BREAKGLASS_SSH_KEY}" = $'-----BEGIN KEY-----\nline2\n-----END KEY-----' ]
}

@test "a break-glass credential passed as a raw env var is purged" {
  export STORAGE_TARGET_1_BREAKGLASS_B2_KEY="raw"
  run_load
  [ -z "${STORAGE_TARGET_1_BREAKGLASS_B2_KEY:-}" ]
}
