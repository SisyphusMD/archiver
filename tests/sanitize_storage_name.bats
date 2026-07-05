#!/usr/bin/env bats
# sanitize_storage_name turns a user storage name into a shell-safe identifier
# fragment used to build DUPLICACY_<NAME>_* env vars. Mis-sanitizing silently
# mis-routes credentials, so its edge cases are pinned here — including the two
# do-spaces field incidents (0.8.10 hyphen, 0.8.11 stderr-redirect).

load 'helpers/load'

setup() {
  load_config_loader
}

@test "hyphenated name is sanitized (0.8.10 do-spaces regression)" {
  run sanitize_storage_name "do-spaces"
  [ "$status" -eq 0 ]
  [ "$output" = "do_spaces" ]
}

@test "name starting with a digit gets an underscore prefix" {
  run sanitize_storage_name "5tb-archive"
  [ "$output" = "_5tb_archive" ]
}

@test "spaces and dots become underscores" {
  run sanitize_storage_name "s3.us east"
  [ "$output" = "s3_us_east" ]
}

@test "an already-valid name is returned unchanged" {
  run sanitize_storage_name "b2primary"
  [ "$output" = "b2primary" ]
}

@test "underscores are preserved" {
  run sanitize_storage_name "my_store"
  [ "$output" = "my_store" ]
}

@test "sanitization is idempotent" {
  local once twice
  once="$(sanitize_storage_name "do-spaces.1")"
  twice="$(sanitize_storage_name "$once")"
  [ "$once" = "$twice" ]
}

@test "log output never leaks into the returned name (0.8.11 stderr-redirect regression)" {
  # auto-restore.sh redefines log_message to echo to STDOUT. sanitize_storage_name
  # must redirect that WARN to stderr, or a caller's name="$(sanitize_storage_name …)"
  # captures the log line and corrupts the DUPLICACY_<NAME>_* vars. Capture stdout
  # only (as the real callers do) to prove the returned name stays clean.
  log_message() { echo "WARN: leaked log line"; }
  local name
  name="$(sanitize_storage_name "do-spaces")"
  [ "$name" = "do_spaces" ]
}
