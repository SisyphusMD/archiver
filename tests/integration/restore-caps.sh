#!/usr/bin/env bash
# Verifies the restore capability warning (Phase 4). An ownership-preserving restore in a
# container WITHOUT CHOWN/FOWNER must log a warning (files would land root-owned) but never
# block; with the caps present, or when ownership is not preserved (-ignore-owner), it must not
# warn. The caller sets EXPECT_WARN to match the container's cap set:
#
#   docker run -i --rm --cap-drop ALL --cap-add DAC_OVERRIDE -e EXPECT_WARN=1 \
#     --entrypoint bash archiver:dev -s < tests/integration/restore-caps.sh
#   docker run -i --rm --cap-drop ALL --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     -e EXPECT_WARN=0 --entrypoint bash archiver:dev -s < tests/integration/restore-caps.sh

# shellcheck disable=SC2034  # RESTORE_FLAGS is read by warn_if_restore_caps_missing (sourced below)
set -uo pipefail

source /opt/archiver/lib/core/common.sh
source_if_not_sourced() { :; }
WARNED=0
log_message() { [ "$1" = "WARN" ] && WARNED=1; echo "[$1] $2"; }
# shellcheck source=/dev/null
source /opt/archiver/lib/features/duplicacy-restore.sh

# Ownership-preserving restore: warns iff the caps are missing (EXPECT_WARN reflects the cap set).
RESTORE_FLAGS=""
WARNED=0
warn_if_restore_caps_missing
[ "${WARNED}" = "${EXPECT_WARN:?EXPECT_WARN must be set}" ] \
  || { echo "FAIL: ownership-preserving warn=${WARNED}, expected ${EXPECT_WARN}"; exit 1; }

# -ignore-owner must never warn, regardless of the cap set.
RESTORE_FLAGS="-ignore-owner"
WARNED=0
warn_if_restore_caps_missing
[ "${WARNED}" = "0" ] || { echo "FAIL: -ignore-owner must never warn"; exit 1; }

echo "=== RESTORE-CAPS OK: ownership-preserving warn=${EXPECT_WARN}; -ignore-owner never warns ==="
