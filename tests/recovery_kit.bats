#!/usr/bin/env bats
# lib/features/recovery-kit.sh unit surface. The pieces that CI's wire tests cannot reach: the
# jq-less JSON field extractor that drives the native B2 API client (B2 has no CI sidecar; a
# parsing regression would only surface on a live deployment), the config-snapshot isolation
# that keeps the kit fingerprint independent of runtime mutations like the prune|retain
# rotation override, and the local placement contract — replace through a fresh inode, never
# destroy a kit that has not been replaced, and never record an unverified one as done.

setup() {
  REPO_ROOT="$(cd "$(dirname "${BATS_TEST_FILENAME}")/.." && pwd)"
  SECRETS_DIR="${BATS_TEST_TMPDIR}/secrets"
  CONFIG_FILE="${BATS_TEST_TMPDIR}/config.sh"
  LOG_DIR="${BATS_TEST_TMPDIR}"
  DEPLOYMENT_DIR="${BATS_TEST_TMPDIR}/deployment"
  mkdir -p "${SECRETS_DIR}"
  : >"${CONFIG_FILE}"
  export SECRETS_DIR CONFIG_FILE LOG_DIR DEPLOYMENT_DIR
}

load_recovery_kit() {
  COMMON_SH_SOURCED=true
  LOGGING_SH_SOURCED=true
  ERROR_SH_SOURCED=true
  source_if_not_sourced() { :; }
  log_message() { :; }
  handle_error() { echo "handle_error: $*" >&2; return 1; }
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-loader.sh"
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/core/config-serialize.sh"
  # shellcheck source=/dev/null
  source "${REPO_ROOT}/lib/features/recovery-kit.sh"
}

arrange_config() {
  SERVICE_DIRECTORIES=(/srv/app)
  STORAGE_TARGET_1_NAME="local"
  STORAGE_TARGET_1_TYPE="local"
  STORAGE_TARGET_1_LOCAL_PATH="/backup"
  STORAGE_PASSWORD="storage-password"
  RSA_PASSPHRASE="passphrase"
  RECOVERY_PASSWORD="recovery-kit-password"
  ROTATE_BACKUPS="true"
}

# A realistic b2_authorize_account response: nested allowed{}, a null bucketId, multiline.
B2_AUTH_JSON='{
  "absoluteMinimumPartSize": 5000000,
  "accountId": "abc123def456",
  "allowed": {
    "bucketId": null,
    "bucketName": null,
    "capabilities": ["listBuckets", "writeFiles"],
    "namePrefix": null
  },
  "apiUrl": "https://api004.backblazeb2.com",
  "authorizationToken": "4_00abc123def456_01d2e3f4a5b6c7d8e9f0a1b2_acct",
  "downloadUrl": "https://f004.backblazeb2.com",
  "s3ApiUrl": "https://s3.us-west-004.backblazeb2.com"
}'

B2_LIST_JSON='{
  "buckets": [
    {
      "accountId": "abc123def456",
      "bucketId": "b2bucketid001122334455",
      "bucketName": "duplicacy-bryantserver",
      "bucketType": "allPrivate"
    }
  ]
}'

@test "recovery_kit_json_field extracts flat string fields from a real-shaped auth response" {
  load_recovery_kit
  [ "$(recovery_kit_json_field "${B2_AUTH_JSON}" "apiUrl")" = "https://api004.backblazeb2.com" ]
  [ "$(recovery_kit_json_field "${B2_AUTH_JSON}" "authorizationToken")" = "4_00abc123def456_01d2e3f4a5b6c7d8e9f0a1b2_acct" ]
  [ "$(recovery_kit_json_field "${B2_AUTH_JSON}" "accountId")" = "abc123def456" ]
}

@test "recovery_kit_json_field returns empty for a null (unquoted) field" {
  load_recovery_kit
  [ -z "$(recovery_kit_json_field "${B2_AUTH_JSON}" "bucketId")" ]
  [ -z "$(recovery_kit_json_field "${B2_AUTH_JSON}" "bucketName")" ]
}

@test "recovery_kit_json_field finds the bucket id in a list_buckets response" {
  load_recovery_kit
  [ "$(recovery_kit_json_field "${B2_LIST_JSON}" "bucketId")" = "b2bucketid001122334455" ]
}

@test "symbolic_mode_to_octal converts ls-style perms and drops special bits" {
  load_recovery_kit
  [ "$(symbolic_mode_to_octal -rwxrwxrwx)" = "777" ]
  [ "$(symbolic_mode_to_octal -rwxr-xr-x)" = "755" ]
  [ "$(symbolic_mode_to_octal -rw-r--r--)" = "644" ]
  [ "$(symbolic_mode_to_octal -rw-r-----)" = "640" ]
  [ "$(symbolic_mode_to_octal -rw-------)" = "600" ]
  [ "$(symbolic_mode_to_octal drwxrwxrwx)" = "777" ]
  # set-uid/gid/sticky are dropped; their execute bit is still reflected.
  [ "$(symbolic_mode_to_octal -rwsr-sr-t)" = "755" ]
  # malformed input yields nothing, so the caller falls back.
  [ -z "$(symbolic_mode_to_octal garbage)" ]
  [ -z "$(symbolic_mode_to_octal '')" ]
}

@test "recovery_kit_mode_grants compares only the group/other read bits" {
  load_recovery_kit
  recovery_kit_mode_grants 777 777
  recovery_kit_mode_grants 644 644
  # a more permissive placement than the reference is fine
  recovery_kit_mode_grants 777 644
  # the reference grants no group/other read, so nothing is required
  recovery_kit_mode_grants 700 600
  recovery_kit_mode_grants 0700 0600
  # an owner-only kit beside a world-readable store is the case that locks the mirror user out
  ! recovery_kit_mode_grants 700 777
  ! recovery_kit_mode_grants 600 644
  ! recovery_kit_mode_grants 640 644
}

@test "recovery_kit_place_local replaces the live file through a fresh inode" {
  load_recovery_kit
  d="${BATS_TEST_TMPDIR}/store"; mkdir -p "$d"
  printf 'config\n' >"$d/config"; chmod 644 "$d/config"
  printf 'old\n' >"$d/kit"; chmod 600 "$d/kit"
  before="$(stat -c '%i' "$d/kit")"
  printf 'new\n' >"${BATS_TEST_TMPDIR}/src"

  recovery_kit_place_local "${BATS_TEST_TMPDIR}/src" "$d/kit" "$d/config"

  [ "$(cat "$d/kit")" = "new" ]
  # a fresh inode is what picks up the directory's inheritable ACLs; an in-place overwrite
  # would keep the original inode and its owner-only state forever
  [ "$before" != "$(stat -c '%i' "$d/kit")" ]
  [ "$(stat -c '%a' "$d/kit")" = "644" ]
  [ ! -e "$d/kit.tmp.$$" ]
}

@test "a placement already carrying the reference's mode is left un-chmod'ed" {
  load_recovery_kit
  d="${BATS_TEST_TMPDIR}/store"; mkdir -p "$d"
  printf 'config\n' >"$d/config"; chmod 600 "$d/config"
  printf 'new\n' >"${BATS_TEST_TMPDIR}/src"; chmod 600 "${BATS_TEST_TMPDIR}/src"
  chmod() { printf '%s\n' "$*" >>"${BATS_TEST_TMPDIR}/chmod.calls"; command chmod "$@"; }

  recovery_kit_place_local "${BATS_TEST_TMPDIR}/src" "$d/kit" "$d/config"

  # On an ACL-backed share the fresh inode's real access comes from the directory's inherited
  # ACL, which is invisible here; a chmod to the mode already shown discards it for nothing.
  [ ! -e "${BATS_TEST_TMPDIR}/chmod.calls" ]
  [ "$(stat -c '%a' "$d/kit")" = "600" ]
}

@test "a placement less readable than the reference is still chmod'ed up to it" {
  load_recovery_kit
  d="${BATS_TEST_TMPDIR}/store"; mkdir -p "$d"
  printf 'config\n' >"$d/config"; chmod 644 "$d/config"
  printf 'new\n' >"${BATS_TEST_TMPDIR}/src"; chmod 600 "${BATS_TEST_TMPDIR}/src"

  recovery_kit_place_local "${BATS_TEST_TMPDIR}/src" "$d/kit" "$d/config"

  [ "$(stat -c '%a' "$d/kit")" = "644" ]
}

@test "a kit that cannot be staged leaves the previous one intact" {
  load_recovery_kit
  d="${BATS_TEST_TMPDIR}/store"; mkdir -p "$d"
  printf 'config\n' >"$d/config"
  printf 'old\n' >"$d/kit"
  # Occupy the staging path with a directory so the copy cannot succeed. Deterministic for root
  # and non-root alike, unlike revoking write permission.
  mkdir -p "$d/kit.tmp.$$"
  printf 'new\n' >"${BATS_TEST_TMPDIR}/src"

  run recovery_kit_place_local "${BATS_TEST_TMPDIR}/src" "$d/kit" "$d/config"

  [ "$status" -eq 1 ]
  # the whole point: never destroy a recovery kit we have not replaced
  [ "$(cat "$d/kit")" = "old" ]
}

@test "an unverified placement is kept out of the uploaded-state record" {
  load_recovery_kit
  arrange_config
  recovery_kit_snapshot_config
  STORAGE_TARGET_COUNT=1
  encrypt_recovery_kit() { : >"${2}"; }
  write_recovery_kit_readme() { : >"${1}"; }
  recovery_kit_upload_to_target() { return "${RECOVERY_KIT_UNVERIFIED}"; }

  run run_recovery_kit

  # Not an upload failure, but it must not read as full success either: the manual wrapper
  # would otherwise announce the kit current on every target.
  [ "$status" -eq 2 ]
  # Recording it as done is what froze an unreadable kit in place — it must be absent so the
  # next run re-places it.
  grep -q '^v' "${LOG_DIR}/.recovery-kit-state"
  ! grep -qx 'local' "${LOG_DIR}/.recovery-kit-state"
}

@test "a kit is not replaced when it cannot be hashed" {
  load_recovery_kit
  d="${BATS_TEST_TMPDIR}/store"; mkdir -p "$d"
  printf 'config\n' >"$d/config"
  printf 'old\n' >"$d/kit"
  printf 'new\n' >"${BATS_TEST_TMPDIR}/src"
  # Both substitutions would expand empty and compare equal without a status check.
  sha256sum() { return 1; }

  run recovery_kit_place_local "${BATS_TEST_TMPDIR}/src" "$d/kit" "$d/config"

  [ "$status" -eq 1 ]
  [ "$(cat "$d/kit")" = "old" ]
  [ ! -e "$d/kit.tmp.$$" ]
}

@test "sftp mode read is strict for verification and fail-open only for stamping" {
  load_recovery_kit
  sftp() { printf '%s\n' "-rwxrwxrwx 1 u g 30 Jan 1 00:00 config"; }
  [ "$(recovery_kit_sftp_mode /d/config host)" = "777" ]

  # A server that reports no usable attributes must not be read as a verified mode: the old
  # 644 fallback satisfied every read-bit check and would pass an unread placement.
  sftp() { printf '\n'; }
  run recovery_kit_sftp_mode /d/config host
  [ "$status" -ne 0 ]
  [ -z "$output" ]
  # ...while choosing what mode to stamp still falls back to a mirror-readable default.
  [ "$(recovery_kit_sftp_ref_mode /d/config host)" = "644" ]
}

@test "a verified placement is recorded so the next run skips it" {
  load_recovery_kit
  arrange_config
  recovery_kit_snapshot_config
  STORAGE_TARGET_COUNT=1
  encrypt_recovery_kit() { : >"${2}"; }
  write_recovery_kit_readme() { : >"${1}"; }
  recovery_kit_upload_to_target() { return 0; }

  run_recovery_kit

  grep -qx 'local' "${LOG_DIR}/.recovery-kit-state"
}

@test "validate_recovery_password rejects short and storage-equal passwords" {
  load_recovery_kit
  arrange_config
  RECOVERY_PASSWORD="short"
  run validate_recovery_password
  [ "$status" -ne 0 ]
  RECOVERY_PASSWORD="${STORAGE_PASSWORD}"
  run validate_recovery_password
  [ "$status" -ne 0 ]
  RECOVERY_PASSWORD="a-good-long-recovery-password"
  validate_recovery_password
}

@test "fingerprint is immune to runtime mutation but tracks real config changes" {
  load_recovery_kit
  arrange_config
  recovery_kit_snapshot_config

  d1="${BATS_TEST_TMPDIR}/p1"; mkdir -p "$d1"
  f1="$(build_recovery_kit_payload "$d1")"

  # Simulate what `archiver backup` + the rotation check do at runtime.
  ROTATE_BACKUPS="false"
  PRUNE_KEEP="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
  d2="${BATS_TEST_TMPDIR}/p2"; mkdir -p "$d2"
  f2="$(build_recovery_kit_payload "$d2")"
  [ "$f1" = "$f2" ]
  grep -q '^ROTATE_BACKUPS=true$' "$d2/archiver.env"
  ! grep -q 'PRUNE_KEEP' "$d2/archiver.env"

  # A genuine config change (new snapshot) must change the fingerprint.
  STORAGE_PASSWORD="rotated-storage-password"
  recovery_kit_snapshot_config
  d3="${BATS_TEST_TMPDIR}/p3"; mkdir -p "$d3"
  f3="$(build_recovery_kit_payload "$d3")"
  [ "$f3" != "$f1" ]
}

@test "the payload is self-contained and carries notes + mounted manifests" {
  load_recovery_kit
  arrange_config
  recovery_kit_snapshot_config
  # ConfigMap-style layout: visible symlink into a hidden ..data dir.
  mkdir -p "${DEPLOYMENT_DIR}/..data"
  printf 'services: {archiver: {image: t}}\n' >"${DEPLOYMENT_DIR}/..data/compose.yaml"
  ln -s ..data/compose.yaml "${DEPLOYMENT_DIR}/compose.yaml"
  d="${BATS_TEST_TMPDIR}/p"; mkdir -p "$d"
  f_with="$(build_recovery_kit_payload "$d")"
  [ "$(cat "$d/secrets/recovery_password")" = "recovery-kit-password" ]
  [ "$(cat "$d/secrets/storage_password")" = "storage-password" ]
  grep -q '/srv/app' "$d/RECREATE.txt"
  grep -q '/backup' "$d/RECREATE.txt"
  cmp -s "$d/deployment/compose.yaml" "${DEPLOYMENT_DIR}/compose.yaml"
  [ -f "$d/deployment/compose.yaml" ] && [ ! -L "$d/deployment/compose.yaml" ]
  [ ! -e "$d/deployment/..data" ]

  # The manifest participates in the change fingerprint.
  printf '# changed\n' >>"${DEPLOYMENT_DIR}/compose.yaml"
  d2="${BATS_TEST_TMPDIR}/p2"; mkdir -p "$d2"
  f_changed="$(build_recovery_kit_payload "$d2")"
  [ "$f_with" != "$f_changed" ]
}
