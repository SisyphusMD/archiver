#!/usr/bin/env bats
# Layered config load in lib/core/config-loader.sh: an optional bundle config.sh baseline,
# overridden by non-secret env vars and file-based secrets. This is what lets a deployment
# migrate off the encrypted bundle one value at a time (env-native / k8s).
#
# Unlike config_loader.bats, these tests must control the environment + fixtures BEFORE the
# load-time orchestration runs, so they arrange state and then source config-loader via
# run_load (rather than the shared setup() sourcing it once).

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  SECRETS_DIR="${BATS_TEST_TMPDIR}/secrets"
  CONFIG_FILE="${BATS_TEST_TMPDIR}/config.sh"
  mkdir -p "${SECRETS_DIR}"
  : >"${CONFIG_FILE}"
  export SECRETS_DIR CONFIG_FILE
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

@test "resolve_secret: no file leaves the bundle value intact" {
  echo 'STORAGE_PASSWORD="from-bundle"' >"${CONFIG_FILE}"
  run_load
  [ "${STORAGE_PASSWORD}" = "from-bundle" ]
}

@test "a secret file overrides the bundle secret" {
  echo 'STORAGE_PASSWORD="from-bundle"' >"${CONFIG_FILE}"
  printf 'from-secret-file' >"${SECRETS_DIR}/storage_password"
  run_load
  [ "${STORAGE_PASSWORD}" = "from-secret-file" ]
}

@test "a secret passed as a raw env var is purged (never trusted)" {
  export STORAGE_PASSWORD="raw-env-secret"
  run_load
  [ -z "${STORAGE_PASSWORD}" ]
}

@test "an env var overrides the bundle non-secret value" {
  echo 'STORAGE_TARGET_1_TYPE="local"' >"${CONFIG_FILE}"
  export STORAGE_TARGET_1_TYPE="s3"
  run_load
  [ "${STORAGE_TARGET_1_TYPE}" = "s3" ]
}

@test "bundle non-secret survives when no env override is set" {
  echo 'ROTATE_BACKUPS="false"' >"${CONFIG_FILE}"
  run_load
  [ "${ROTATE_BACKUPS}" = "false" ]
}

@test "pure env-native: no bundle file, config comes entirely from env + secrets" {
  rm -f "${CONFIG_FILE}"
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

@test "normalize_service_directories: a legacy bundle array is left untouched" {
  printf 'SERVICE_DIRECTORIES=(\n  "/x/"\n  "/y/"\n)\n' >"${CONFIG_FILE}"
  run_load
  [ "${#SERVICE_DIRECTORIES[@]}" -eq 2 ]
  [ "${SERVICE_DIRECTORIES[0]}" = "/x/" ]
  [ "${SERVICE_DIRECTORIES[1]}" = "/y/" ]
}
