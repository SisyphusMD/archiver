#!/usr/bin/env bats
# format_duration in lib/core/logging.sh feeds every completion/stop notification and the
# session summary; pure arithmetic, so pin the singular/plural and list-joining rules.

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  COMMON_SH_SOURCED=true
  ERROR_SH_SOURCED=true
  NOTIFICATION_SH_SOURCED=true
  source_if_not_sourced() { :; }
  handle_error() { :; }
  notify() { :; }
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/logging.sh"
}

@test "0 seconds" {
  [ "$(format_duration 0)" = "0 seconds" ]
}

@test "singular second" {
  [ "$(format_duration 1)" = "1 second" ]
}

@test "plural seconds" {
  [ "$(format_duration 59)" = "59 seconds" ]
}

@test "exact minute drops the seconds part" {
  [ "$(format_duration 60)" = "1 minute" ]
}

@test "two parts join with 'and'" {
  [ "$(format_duration 61)" = "1 minute and 1 second" ]
}

@test "three parts use commas plus 'and'" {
  [ "$(format_duration 3661)" = "1 hour, 1 minute, and 1 second" ]
}

@test "exact day" {
  [ "$(format_duration 86400)" = "1 day" ]
}

@test "four parts, all singular" {
  [ "$(format_duration 90061)" = "1 day, 1 hour, 1 minute, and 1 second" ]
}

@test "plural days" {
  [ "$(format_duration 180122)" = "2 days, 2 hours, 2 minutes, and 2 seconds" ]
}
