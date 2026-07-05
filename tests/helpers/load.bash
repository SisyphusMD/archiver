#!/usr/bin/env bash
# Shared bats setup. config-loader.sh has load-time side effects — it sources
# common.sh, logging.sh, and $CONFIG_FILE — so to unit-test its functions in
# isolation we pre-satisfy those guards with stubs and hand it an empty config.

REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"

load_config_loader() {
  COMMON_SH_SOURCED=true          # skip the hardcoded /opt/archiver/.../common.sh source (line 7 guard)
  LOGGING_SH_SOURCED=true         # skip logging.sh (guard read by source_if_not_sourced)
  source_if_not_sourced() { :; }  # provided by common.sh in production
  log_message() { :; }            # provided by logging.sh; default no-op stub
  handle_error() { echo "handle_error: $*" >&2; return 1; }

  CONFIG_FILE="${BATS_TEST_TMPDIR}/config.sh"
  : >"${CONFIG_FILE}"

  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-loader.sh"
}
