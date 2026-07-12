#!/bin/bash
# Storage maintenance: per-storage check + prune, run by the maintenance pipeline
# (lib/scripts/maintenance.sh) independently of backups. Holds the per-storage lock while
# working on a storage, so it never overlaps a copy that touches that storage: the copy
# phase holds both its source (the primary it reads from) and each destination lock, and
# copy-vs-prune concurrency is not documented as safe by duplicacy. backup-vs-prune IS safe,
# so a backup writing to the primary is never blocked — only a copy reading from it is.

DUPLICACY_MAINTENANCE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
source_if_not_sourced "${LOCKFILE_CORE}"
DUPLICACY_BIN="duplicacy"

# State: one line per storage — "<name> <last_check_ok> <last_prune_ok> <last_exhaustive_ok>"
# (epoch seconds, 0 = never). Lives in LOG_DIR so it survives container recreation; also
# read by healthcheck for staleness warnings.
MAINTENANCE_STATE_FILE="${LOG_DIR}/.maintenance-state"

get_maintenance_state() {
  local storage="${1}" field="${2}"                # field: 2=check 3=prune 4=exhaustive
  local line
  line="$(grep "^${storage} " "${MAINTENANCE_STATE_FILE}" 2>/dev/null | head -1)"
  [ -n "${line}" ] || { echo 0; return; }
  echo "${line}" | cut -d' ' -f"${field}"
}

set_maintenance_state() {
  local storage="${1}" field="${2}" value="${3}"
  local check prune exhaustive
  check="$(get_maintenance_state "${storage}" 2)"
  prune="$(get_maintenance_state "${storage}" 3)"
  exhaustive="$(get_maintenance_state "${storage}" 4)"
  case "${field}" in
    2) check="${value}" ;;
    3) prune="${value}" ;;
    4) exhaustive="${value}" ;;
  esac
  {
    grep -v "^${storage} " "${MAINTENANCE_STATE_FILE}" 2>/dev/null
    echo "${storage} ${check:-0} ${prune:-0} ${exhaustive:-0}"
  } > "${MAINTENANCE_STATE_FILE}.tmp"
  mv "${MAINTENANCE_STATE_FILE}.tmp" "${MAINTENANCE_STATE_FILE}"
  chmod 600 "${MAINTENANCE_STATE_FILE}"
}

# Is an exhaustive prune due for this storage? An hour of grace keeps a fixed daily
# schedule from drifting one slot later each cycle (a 30-day interval measured against a
# 30-day-minus-runtime gap would otherwise never quite be "due" on the natural day).
exhaustive_due() {
  local storage="${1}"
  local last now interval
  [ "${FORCE_EXHAUSTIVE:-false}" = "true" ] && return 0
  case "${PRUNE_EXHAUSTIVE_FREQUENCY}" in
    off) return 1 ;;
    daily) interval=86400 ;;
    weekly) interval=604800 ;;
    monthly) interval=2592000 ;;
    *) return 1 ;;
  esac
  last="$(get_maintenance_state "${storage}" 4)"
  now="$(date +%s)"
  [ $(( now - last )) -ge $(( interval - 3600 )) ]
}

# Run one duplicacy command stoppably: process substitution keeps duplicacy a direct
# child (stop signals children via pkill -P), same pattern as duplicacy_primary_backup.
run_duplicacy_stoppable() {
  local exit_status
  "${DUPLICACY_BIN}" "$@" > >(log_output) 2>&1 &
  local cmd_pid=$!

  while kill -0 "${cmd_pid}" 2>/dev/null; do
    if is_stop_requested; then
      log_message "INFO" "Stop requested during duplicacy ${1}."
      pkill -TERM -P "${cmd_pid}" 2>/dev/null || true
      kill -TERM "${cmd_pid}" 2>/dev/null || true
      sleep 2
      if kill -0 "${cmd_pid}" 2>/dev/null; then
        pkill -KILL -P "${cmd_pid}" 2>/dev/null || true
        kill -KILL "${cmd_pid}" 2>/dev/null || true
      fi
      wait "${cmd_pid}" 2>/dev/null || true
      return 130
    fi
    sleep 1
  done

  wait "${cmd_pid}"
  return $?
}

# Full maintenance pass for one storage-target id: check -> prune. Runs LOCK-FREE against the
# storage — duplicacy's two-step fossil collection makes a non-exclusive prune/check safe
# alongside a concurrent copy, so the pipelines no longer serialize per storage.
# Phase timings are logged; last-success timestamps recorded for healthcheck staleness.
maintain_storage() {
  local storage_id="${1}"
  local storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
  local storage_name exit_status phase_start
  storage_name="$(sanitize_storage_name "${!storage_name_var}")"

  SERVICE="${storage_name}"

  export_duplicacy_storage_secrets "${storage_id}"

  if [[ "${CHECK_BACKUPS}" == "true" ]]; then
    update_lock_stage "storage:${storage_name}" "check"
    phase_start="$(date +%s)"
    run_duplicacy_stoppable check -all -storage "${storage_name}" -fossils -resurrect -stats -threads "${DUPLICACY_THREADS}"
    exit_status=$?
    if [ "${exit_status}" -eq 130 ]; then
      unset SERVICE; return 130
    elif [ "${exit_status}" -ne 0 ]; then
      handle_error "Storage check failed for ${storage_name}."
    else
      set_maintenance_state "${storage_name}" 2 "$(date +%s)"
      log_message "INFO" "Storage check completed for ${storage_name} in $(format_duration $(( $(date +%s) - phase_start )))."
    fi
  fi

  if is_stop_requested; then
    unset SERVICE; return 130
  fi

  if [[ "${PRUNE_BACKUPS}" == "true" ]]; then
    local prune_args=()
    declare -a PRUNE_KEEP_ARRAY
    read -r -a PRUNE_KEEP_ARRAY <<< "${PRUNE_KEEP}"
    prune_args=(prune -all -storage "${storage_name}" "${PRUNE_KEEP_ARRAY[@]}" -threads "${DUPLICACY_THREADS}")
    local exhaustive_run=false
    if exhaustive_due "${storage_name}"; then
      prune_args+=(-exhaustive)
      exhaustive_run=true
      log_message "INFO" "Exhaustive prune due for ${storage_name} (frequency: ${PRUNE_EXHAUSTIVE_FREQUENCY})."
    fi

    update_lock_stage "storage:${storage_name}" "prune"
    phase_start="$(date +%s)"
    run_duplicacy_stoppable "${prune_args[@]}"
    exit_status=$?
    if [ "${exit_status}" -eq 130 ]; then
      unset SERVICE; return 130
    elif [ "${exit_status}" -ne 0 ]; then
      handle_error "Prune failed for ${storage_name} storage. Review the Duplicacy logs for details."
    else
      set_maintenance_state "${storage_name}" 3 "$(date +%s)"
      [ "${exhaustive_run}" = "true" ] && set_maintenance_state "${storage_name}" 4 "$(date +%s)"
      log_message "INFO" "Prune completed for ${storage_name} storage in $(format_duration $(( $(date +%s) - phase_start )))."
    fi
  fi

  unset SERVICE
  return 0
}
