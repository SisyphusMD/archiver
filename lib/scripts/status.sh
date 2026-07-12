#!/bin/bash

STATUS_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"

# Small local humanizer (this script deliberately stays off the logging chain).
age_of() {
  local epoch="${1}" now delta
  [ -z "${epoch}" ] || [ "${epoch}" = "0" ] && { echo "never"; return; }
  now="$(date +%s)"
  delta=$(( now - epoch ))
  if [ "${delta}" -lt 3600 ]; then echo "$(( delta / 60 ))m ago"
  elif [ "${delta}" -lt 172800 ]; then echo "$(( delta / 3600 ))h ago"
  else echo "$(( delta / 86400 ))d ago"
  fi
}

# Backup pipeline
if is_lock_valid; then
  LOCK_PID=$(get_lock_pid)
  if is_paused; then
    echo "Backup: paused (PID: ${LOCK_PID})."
  else
    echo "Backup: running (PID: ${LOCK_PID}, stage: $(get_lock_stage))."
  fi
else
  echo "Backup: not running."
fi

# Maintenance pipeline
(
  ARCHIVER_LOCKFILE="${MAINTENANCE_LOCKFILE}"
  ARCHIVER_STOP_FLAG_FILE="${MAINTENANCE_STOP_FLAG}"
  if is_lock_valid; then
    echo "Maintenance: running (PID: $(get_lock_pid), stage: $(get_lock_stage), $(get_lock_context))."
  else
    echo "Maintenance: not running."
  fi
)

# Per-storage maintenance recency (written by the maintenance pipeline).
STATE_FILE="${LOG_DIR}/.maintenance-state"
if [ -s "${STATE_FILE}" ]; then
  echo "Storage maintenance (last success):"
  while read -r name check prune exhaustive; do
    [ -n "${name}" ] || continue
    echo "  ${name}: check $(age_of "${check}"), prune $(age_of "${prune}"), exhaustive $(age_of "${exhaustive}")"
  done < "${STATE_FILE}"
fi

exit 0
