#!/bin/bash
#
# Archiver Health Check Script
# Performs comprehensive health checks for monitoring systems
#
# Exit codes:
#   0 - Healthy
#   1 - Unhealthy

# Archiver directory
ARCHIVER_DIR="/opt/archiver"

ERRORS=0
WARNINGS=0

# Helper functions
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

# Health Check 1: Configuration exists
if [ ! -f "${ARCHIVER_DIR}/config.sh" ]; then
  error "Configuration file not found (config.sh)"
else
  info "Configuration file exists"
fi

# Health Check 2: Required binaries are accessible
if ! command -v duplicacy >/dev/null 2>&1; then
  error "Duplicacy binary not found in PATH"
else
  info "Duplicacy binary accessible"
fi

# Health Check 3: Required keys exist
if [ ! -f "${ARCHIVER_DIR}/keys/private.pem" ]; then
  error "RSA private key not found"
else
  info "RSA private key exists"
fi

if [ ! -f "${ARCHIVER_DIR}/keys/id_ed25519" ]; then
  warn "SSH private key not found (OK if not using SFTP)"
else
  info "SSH private key exists"
fi

# Health Check 4: Log file exists and is recent
LOG_FILE="${ARCHIVER_DIR}/logs/archiver.log"
if [ ! -f "${LOG_FILE}" ]; then
  warn "Log file not found (may not have run yet)"
else
  # Check if log file has been modified in the last 48 hours
  # This is reasonable for daily backups with some margin
  if [ -n "$(find "${LOG_FILE}" -mmin -2880 2>/dev/null)" ]; then
    info "Log file is recent (modified within 48 hours)"
  else
    warn "Log file is stale (not modified in 48 hours)"
  fi

  # Check for recent errors in last 100 lines
  if [ -f "${LOG_FILE}" ]; then
    ERROR_COUNT=$(tail -100 "${LOG_FILE}" 2>/dev/null | grep -c "\[ERROR\]" || true)
    if [ "${ERROR_COUNT}" -gt 0 ]; then
      # Check if backup completed despite errors (transient errors are OK)
      if tail -100 "${LOG_FILE}" 2>/dev/null | grep -q "Archiver script completed"; then
        warn "Found ${ERROR_COUNT} errors in recent logs, but backup completed"
      else
        error "Found ${ERROR_COUNT} errors in recent logs without completion"
      fi
    else
      info "No errors in recent logs"
    fi
  fi
fi

# Health Check 5: Check for stale lockfile (indicates stuck backup)
LOCKFILE_PATTERN="/var/lock/archiver-*.lock"
if ls ${LOCKFILE_PATTERN} >/dev/null 2>&1; then
  for lockfile in ${LOCKFILE_PATTERN}; do
    if [ -f "$lockfile" ]; then
      # Check if lockfile is older than 24 hours
      if [ -n "$(find "$lockfile" -mmin +1440 2>/dev/null)" ]; then
        error "Stale lockfile detected (older than 24 hours): $lockfile"
      else
        info "Backup is currently running (lockfile present)"
      fi
    fi
  done
else
  info "No active backup (no lockfile)"
fi

# Health Check 6: Verify disk space for logs
LOG_DIR="${ARCHIVER_DIR}/logs"
if [ -d "${LOG_DIR}" ]; then
  # Get available disk space in MB
  AVAILABLE_MB=$(df -BM "${LOG_DIR}" | awk 'NR==2 {print $4}' | sed 's/M//')
  if [ "${AVAILABLE_MB}" -lt 100 ]; then
    error "Low disk space for logs (${AVAILABLE_MB}MB available)"
  else
    info "Sufficient disk space for logs (${AVAILABLE_MB}MB available)"
  fi
fi

# Summary
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
