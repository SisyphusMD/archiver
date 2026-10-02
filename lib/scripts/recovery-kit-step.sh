#!/bin/bash
# The Go backup pipeline's recovery-kit step while the kit is still bash. Exits 0, 1 for
# errors (each already logged and notified), 2 when the kit was placed but a target could
# not be verified readable (never counted as an error), or 3 when only secondary uploads
# failed (the copy workers report those storages).

# shellcheck disable=SC2034  # source_if_not_sourced gates on this
RECOVERY_KIT_STEP_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${RECOVERY_KIT_FEATURE}"

# count_storage_targets would log the count a second time in the same run.
STORAGE_TARGET_COUNT=0
while v="STORAGE_TARGET_$((STORAGE_TARGET_COUNT + 1))_NAME"; [[ -n "${!v:-}" ]]; do
  STORAGE_TARGET_COUNT=$((STORAGE_TARGET_COUNT + 1))
done
export STORAGE_TARGET_COUNT

run_recovery_kit
