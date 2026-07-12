#!/usr/bin/env bats
# Config validation logic in lib/core/config-loader.sh: storage-target counting
# and required-secret checks. These gate every backup/restore run.

load 'helpers/load'

setup() {
  load_config_loader
}

@test "count_storage_targets counts contiguous targets" {
  STORAGE_TARGET_1_NAME="a"
  STORAGE_TARGET_2_NAME="b"
  count_storage_targets
  [ "$STORAGE_TARGET_COUNT" -eq 2 ]
}

@test "count_storage_targets stops at the first gap" {
  STORAGE_TARGET_1_NAME="a"
  # _2 intentionally unset; _3 set — the counter stops at the gap, not counting _3
  STORAGE_TARGET_3_NAME="c"
  count_storage_targets
  [ "$STORAGE_TARGET_COUNT" -eq 1 ]
}

@test "count_storage_targets errors when none are configured" {
  run count_storage_targets
  [ "$status" -ne 0 ]
}

@test "check_required_secrets fails when RSA_PASSPHRASE is missing" {
  STORAGE_PASSWORD="x"
  RSA_PASSPHRASE=""
  run check_required_secrets
  [ "$status" -ne 0 ]
}

@test "check_required_secrets passes when both secrets are set" {
  STORAGE_PASSWORD="longenoughpw"
  RSA_PASSPHRASE="y"
  run check_required_secrets
  [ "$status" -eq 0 ]
}

@test "check_required_secrets fails when STORAGE_PASSWORD is shorter than 8 chars" {
  STORAGE_PASSWORD="short"
  RSA_PASSPHRASE="y"
  run check_required_secrets
  [ "$status" -ne 0 ]
}

@test "check_maintenance_settings rejects an invalid PRUNE_EXHAUSTIVE_FREQUENCY" {
  PRUNE_EXHAUSTIVE_FREQUENCY="fortnightly"
  PRUNE_KEEP=""
  run check_maintenance_settings
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "must be one of"
}

@test "check_maintenance_settings applies defaults when unset" {
  PRUNE_EXHAUSTIVE_FREQUENCY=""
  PRUNE_KEEP=""
  CHECK_BACKUPS=""
  PRUNE_BACKUPS=""
  check_maintenance_settings
  [ "${PRUNE_EXHAUSTIVE_FREQUENCY}" = "monthly" ]
  [ "${CHECK_BACKUPS}" = "true" ]
  [ "${PRUNE_BACKUPS}" = "true" ]
  [ "${PRUNE_KEEP}" = "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1" ]
}

@test "check_maintenance_settings lowercases toggle and frequency values" {
  CHECK_BACKUPS="TRUE"
  PRUNE_BACKUPS="False"
  PRUNE_EXHAUSTIVE_FREQUENCY="WEEKLY"
  PRUNE_KEEP="-keep 0:1"
  check_maintenance_settings
  [ "${CHECK_BACKUPS}" = "true" ]
  [ "${PRUNE_BACKUPS}" = "false" ]
  [ "${PRUNE_EXHAUSTIVE_FREQUENCY}" = "weekly" ]
}
