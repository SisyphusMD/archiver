#!/bin/bash
# `archiver recovery-kit [force]` — build the encrypted recovery kit and place it on every
# storage target now. Normally this runs automatically after each backup; the manual
# command exists for first-time setup and for re-pushing with `force`.

# Distinct guard name: the recovery-kit FEATURE shares this basename, and
# source_if_not_sourced derives its guard from the basename — reusing it here would skip
# sourcing the feature.
RECOVERY_KIT_SCRIPT_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_LOADER_CORE}"      # loads the effective configuration
source_if_not_sourced "${RECOVERY_KIT_FEATURE}"

if ! recovery_kit_configured; then
  echo "The recovery kit is not configured. Provide the recovery password at ${SECRETS_DIR}/recovery_password (or point RECOVERY_PASSWORD_FILE at it)." >&2
  exit 1
fi

count_storage_targets
verify_target_settings
check_required_secrets

if run_recovery_kit "${1:-}"; then
  echo "Recovery kit complete: $(recovery_kit_file_name) is current on all storage targets."
else
  echo "Recovery kit finished with errors; see the log. Failed targets will be retried on the next run." >&2
  exit 1
fi
