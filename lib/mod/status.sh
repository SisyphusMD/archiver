#!/bin/bash

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

# Determine archiver repo directory path by traversing up the directory tree until we find 'archiver.sh' or reach the root
STATUS_SCRIPT_PATH="$(realpath "$0")"
CURRENT_DIR="$(dirname "${STATUS_SCRIPT_PATH}")"
ARCHIVER_DIR=""
while [ "${CURRENT_DIR}" != "/" ]; do
  if [ -f "${CURRENT_DIR}/archiver.sh" ]; then
    ARCHIVER_DIR="${CURRENT_DIR}"
    break
  fi
  CURRENT_DIR="$(dirname "${CURRENT_DIR}")"
done

# Check if we found the file
if [ -z "${ARCHIVER_DIR}" ]; then
  echo "Error: archiver.sh not found in any parent directory."
  exit 1
fi

# Define lib, src, mod, log, logo directories
LIB_DIR="${ARCHIVER_DIR}/lib"
MOD_DIR="${LIB_DIR}/mod"

# Define unique identifier for the main script (e.g., main script's full path)
MAIN_SCRIPT_PATH="${MOD_DIR}/main.sh"
LOCKFILE="/var/lock/archiver-$(echo "${MAIN_SCRIPT_PATH}" | md5sum | cut -d' ' -f1).lock"

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_INFO="$(cat "${LOCKFILE}")"
  LOCK_PID="$(echo "${LOCK_INFO}" | cut -d' ' -f1)"
  LOCK_SCRIPT="$(echo "${LOCK_INFO}" | cut -d' ' -f2)"

  if [ -n "${LOCK_PID}" ] && [ "${LOCK_SCRIPT}" = "${MAIN_SCRIPT_PATH}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    # This means PID exists
    state="$(awk '/State/ {print $3}' /proc/"${LOCK_PID}"/status)"
    if [ "${state}" == "(running)" ] || [ "${state}" == "(sleeping)" ]; then
        echo "An Archiver backup is running."
    elif [ "${state}" == "(stopped)" ]; then
        echo "An Archiver backup is paused."
    else
        echo "Process state is '${state}'."
    fi
  else
    echo "Stale lock file detected. No running Archiver backup found with PID ${LOCK_PID}. Run 'archiver stop' to fix this."
  fi
else
  echo "Archiver backup is not running."
fi
