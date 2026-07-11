#!/usr/bin/env bats
# lib/features/recovery-kit.sh unit surface. The two pieces that CI's wire tests cannot
# reach: the jq-less JSON field extractor that drives the native B2 API client (B2 has no
# CI sidecar; a parsing regression would only surface on a live deployment), and the
# config-snapshot isolation that keeps the kit fingerprint independent of runtime
# mutations like the prune|retain rotation override.

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

  # Simulate what `archiver backup retain` + the rotation check do at runtime.
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
