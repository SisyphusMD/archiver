#!/usr/bin/env bats
# expand_service_directories in lib/core/config-loader.sh: glob expansion of
# SERVICE_DIRECTORIES into EXPANDED_SERVICE_DIRECTORIES (what main.sh iterates).

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  SECRETS_DIR="${BATS_TEST_TMPDIR}/secrets"
  CONFIG_FILE="${BATS_TEST_TMPDIR}/config.sh"
  mkdir -p "${SECRETS_DIR}"
  : >"${CONFIG_FILE}"
  export SECRETS_DIR CONFIG_FILE
  SVC="${BATS_TEST_TMPDIR}/svc"
  mkdir -p "${SVC}/app1" "${SVC}/app2" "${SVC}/other"
}

# Same arrange-then-source pattern as layered_config.bats.
run_load() {
  COMMON_SH_SOURCED=true
  LOGGING_SH_SOURCED=true
  source_if_not_sourced() { :; }
  log_message() { :; }
  handle_error() { echo "handle_error: $*" >&2; return 1; }
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-loader.sh"
}

@test "a trailing-slash glob expands to every directory, slashes stripped" {
  export SERVICE_DIRECTORIES="${SVC}/*/"
  run_load
  expand_service_directories
  [ "${#EXPANDED_SERVICE_DIRECTORIES[@]}" -eq 3 ]
  [ "${EXPANDED_SERVICE_DIRECTORIES[0]}" = "${SVC}/app1" ]
  [ "${EXPANDED_SERVICE_DIRECTORIES[1]}" = "${SVC}/app2" ]
  [ "${EXPANDED_SERVICE_DIRECTORIES[2]}" = "${SVC}/other" ]
}

@test "plain files matched by a pattern are skipped (directories only)" {
  touch "${SVC}/app1/notadir.txt"
  export SERVICE_DIRECTORIES="${SVC}/app1/*"
  run_load
  expand_service_directories
  [ "${#EXPANDED_SERVICE_DIRECTORIES[@]}" -eq 0 ]
}

@test "colon-delimited entries expand independently (glob + literal mix)" {
  export SERVICE_DIRECTORIES="${SVC}/app*/:${SVC}/other/"
  run_load
  expand_service_directories
  [ "${#EXPANDED_SERVICE_DIRECTORIES[@]}" -eq 3 ]
  [ "${EXPANDED_SERVICE_DIRECTORIES[2]}" = "${SVC}/other" ]
}

@test "a pattern matching nothing contributes nothing (no literal-glob entry)" {
  export SERVICE_DIRECTORIES="${SVC}/nomatch*/:${SVC}/app1/"
  run_load
  expand_service_directories
  [ "${#EXPANDED_SERVICE_DIRECTORIES[@]}" -eq 1 ]
  [ "${EXPANDED_SERVICE_DIRECTORIES[0]}" = "${SVC}/app1" ]
}

@test "unset SERVICE_DIRECTORIES is a hard error" {
  unset SERVICE_DIRECTORIES
  run_load
  run expand_service_directories
  [ "${status}" -eq 1 ]
  [[ "${output}" == *"SERVICE_DIRECTORIES is not set"* ]]
}
