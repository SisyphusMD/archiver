#!/bin/bash
# The Go recovery kit's envelope check while the envelope is still bash: records what a
# printed envelope would say now (for status) and notifies once when a confirmed one no
# longer matches or is due a check.

# shellcheck disable=SC2034  # source_if_not_sourced gates on this
ENVELOPE_CHECK_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${ENVELOPE_FEATURE}"

# count_storage_targets would log the count into the backup's log.
STORAGE_TARGET_COUNT=0
while v="STORAGE_TARGET_$((STORAGE_TARGET_COUNT + 1))_NAME"; [[ -n "${!v:-}" ]]; do
  STORAGE_TARGET_COUNT=$((STORAGE_TARGET_COUNT + 1))
done
export STORAGE_TARGET_COUNT

envelope_check
