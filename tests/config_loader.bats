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
  STORAGE_PASSWORD="x"
  RSA_PASSPHRASE="y"
  run check_required_secrets
  [ "$status" -eq 0 ]
}
