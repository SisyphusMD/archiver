#!/bin/bash

HEALTHCHECK_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
ERRORS=0
WARNINGS=0

error() {
  echo "ERROR: $1"
  ERRORS=$((ERRORS + 1))
}

warn() {
  echo "WARNING: $1"
  WARNINGS=$((WARNINGS + 1))
}

info() {
  echo "OK: $1"
}

# Env-native deployments have no config.sh at all (config comes from env vars + secret
# files), so its absence is only an error when there is no RSA key either.
if [ -f "${CONFIG_FILE}" ]; then
  info "Configuration file exists"
elif [ -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ]; then
  info "No config.sh; env-native configuration (RSA key present)"
else
  error "No configuration found (no config.sh and no RSA private key)"
fi

if [ ! -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ]; then
  error "RSA private key not found"
else
  info "RSA private key exists"
fi

if [ ! -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ]; then
  warn "SSH private key not found (OK if not using SFTP)"
else
  info "SSH private key exists"
fi

if [ ! -f "${LOG_DIR}/archiver.log" ]; then
  warn "Log file not found (may not have run yet)"
else
  # Check if log file has been modified in the last 48 hours
  # This is reasonable for daily backups with some margin
  if [ -n "$(find "${LOG_DIR}/archiver.log" -mmin -2880 2>/dev/null)" ]; then
    info "Log file is recent (modified within 48 hours)"
  else
    warn "Log file is stale (not modified in 48 hours)"
  fi

  if [ -f "${LOG_DIR}/archiver.log" ]; then
    ERROR_COUNT=$(tail -100 "${LOG_DIR}/archiver.log" 2>/dev/null | grep -c "\[ERROR\]" || true)
    if [ "${ERROR_COUNT}" -gt 0 ]; then
      # Errors are only fatal when the run never finished: "Main backup script exited."
      # is main.sh's EXIT-trap line, so errors without it mean a crash or hang mid-backup.
      if tail -100 "${LOG_DIR}/archiver.log" 2>/dev/null | grep -q "Main backup script exited."; then
        warn "Found ${ERROR_COUNT} errors in recent logs, but the run finished"
      else
        error "Found ${ERROR_COUNT} errors in recent logs without a finished run"
      fi
    else
      info "No errors in recent logs"
    fi
  fi
fi

if [ -f "${LOG_DIR}/maintenance.log" ]; then
  MAINT_ERROR_COUNT=$(tail -100 "${LOG_DIR}/maintenance.log" 2>/dev/null | grep -c "\[ERROR\]" || true)
  if [ "${MAINT_ERROR_COUNT}" -gt 0 ]; then
    MAINT_PID="$(head -n 1 "${MAINTENANCE_LOCKFILE}" 2>/dev/null | cut -d' ' -f1)"
    if tail -100 "${LOG_DIR}/maintenance.log" 2>/dev/null | grep -q "Maintenance script exited."; then
      warn "Found ${MAINT_ERROR_COUNT} errors in recent maintenance logs, but the run finished"
    elif [ -n "${MAINT_PID}" ] && kill -0 "${MAINT_PID}" 2>/dev/null; then
      # rotate_logs starts each run's log fresh, so the previous run's exit marker is never in
      # this window while a run is live. A mid-run error (one slow storage, a long prune still
      # ahead) is not a failure — the run is still progressing.
      warn "Found ${MAINT_ERROR_COUNT} errors in maintenance logs; run still in progress"
    else
      error "Found ${MAINT_ERROR_COUNT} errors in recent maintenance logs without a finished run"
    fi
  else
    info "No errors in recent maintenance logs"
  fi
fi

# Locks are judged by holder liveness, not age: a multi-day copy or exhaustive prune is
# legitimate, while any dead-PID lock is stale regardless of age. A stale pipeline lock is
# self-healing (acquire_lock reaps it on the next run, like a storage lock), and the entrypoint
# clears all locks at boot — so it is only a warning, not an UNHEALTHY-flipping error.
check_lock_liveness() {
  local path="$1" label="$2"
  [ -f "$path" ] || return 0
  local pid
  pid="$(head -n 1 "$path" 2>/dev/null | cut -d' ' -f1)"
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    info "${label} is currently running (PID ${pid})"
  else
    warn "Stale ${label} lock (will be reaped on next run): ${path}"
  fi
}
check_lock_liveness "${LOCKFILE}" "Backup"
check_lock_liveness "${MAINTENANCE_LOCKFILE}" "Maintenance"

# Maintenance staleness: the maintenance pipeline records per-storage last-success times.
# 8 days covers daily and weekly cadences with margin; if you maintain less often than
# weekly, expect (and ignore) these warnings or run 'archiver maintenance' manually.
MAINT_STATE="${LOG_DIR}/.maintenance-state"
# Bundle-mode deployments keep these toggles in config.sh, not the container env, so read that
# too — otherwise a documented shared-storage bundle (PRUNE_BACKUPS="false") would default to
# true here and warn about a prune that correctly never runs. Env still wins: capture it first,
# source config.sh (which overwrites), then restore any value that was set in the environment.
CHECK_BACKUPS_ENV="${CHECK_BACKUPS:-}"
PRUNE_BACKUPS_ENV="${PRUNE_BACKUPS:-}"
ROTATE_BACKUPS_ENV="${ROTATE_BACKUPS:-}"
# shellcheck source=/dev/null
[ -f "${CONFIG_FILE}" ] && source "${CONFIG_FILE}"
[ -n "${CHECK_BACKUPS_ENV}" ] && CHECK_BACKUPS="${CHECK_BACKUPS_ENV}"
[ -n "${PRUNE_BACKUPS_ENV}" ] && PRUNE_BACKUPS="${PRUNE_BACKUPS_ENV}"
[ -n "${ROTATE_BACKUPS_ENV}" ] && ROTATE_BACKUPS="${ROTATE_BACKUPS_ENV}"
CHECK_TOGGLE="$(echo "${CHECK_BACKUPS:-true}" | tr '[:upper:]' '[:lower:]')"
PRUNE_TOGGLE="$(echo "${PRUNE_BACKUPS:-${ROTATE_BACKUPS:-true}}" | tr '[:upper:]' '[:lower:]')"
if [ "${CHECK_TOGGLE}" = "true" ] || [ "${PRUNE_TOGGLE}" = "true" ]; then
  NOW="$(date +%s)"
  STALE_AFTER=$(( 8 * 86400 ))
  if [ -s "${MAINT_STATE}" ]; then
    while read -r name last_check last_prune _; do
      [ -n "${name}" ] || continue
      if [ "${CHECK_TOGGLE}" = "true" ] && [ $(( NOW - ${last_check:-0} )) -gt "${STALE_AFTER}" ]; then
        warn "No successful check on '${name}' in over 8 days"
      fi
      if [ "${PRUNE_TOGGLE}" = "true" ] && [ $(( NOW - ${last_prune:-0} )) -gt "${STALE_AFTER}" ]; then
        warn "No successful prune on '${name}' in over 8 days"
      fi
    done < "${MAINT_STATE}"
  elif [ -f "${LOG_DIR}/archiver.log" ]; then
    # Backups have run but maintenance never has: retention/verification is not happening.
    warn "Maintenance has never completed (set MAINTENANCE_SCHEDULE or run 'archiver maintenance')"
  fi
fi

if [ -d "${LOG_DIR}" ]; then
  AVAILABLE_MB=$(df -BM "${LOG_DIR}" | awk 'NR==2 {print $4}' | sed 's/M//')
  if [ "${AVAILABLE_MB}" -lt 100 ]; then
    error "Low disk space for logs (${AVAILABLE_MB}MB available)"
  else
    info "Sufficient disk space for logs (${AVAILABLE_MB}MB available)"
  fi
fi

echo ""
echo "=== Health Check Summary ==="
echo "Errors:   ${ERRORS}"
echo "Warnings: ${WARNINGS}"

if [ ${ERRORS} -gt 0 ]; then
  echo "Status:   UNHEALTHY"
  exit 1
elif [ ${WARNINGS} -gt 0 ]; then
  echo "Status:   HEALTHY (with warnings)"
  exit 0
else
  echo "Status:   HEALTHY"
  exit 0
fi
