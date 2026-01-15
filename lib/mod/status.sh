#!/bin/bash

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

command="${1}"

# Archiver directory
ARCHIVER_DIR="/opt/archiver"

# Define lib, src, mod, log, logo directories
LIB_DIR="${ARCHIVER_DIR}/lib"
MOD_DIR="${LIB_DIR}/mod"

# Define unique identifier for the main script (e.g., main script's full path)
MAIN_SCRIPT_PATH="${MOD_DIR}/main.sh"
LOCKFILE="/var/lock/archiver-$(echo "${MAIN_SCRIPT_PATH}" | md5sum | cut -d' ' -f1).lock"
PAUSED_FLAG="paused"

# Function to check if the process is paused
is_paused() {
  grep -q "${PAUSED_FLAG}" "${LOCKFILE}"
}

# Function to set the paused state in the LOCKFILE
set_paused_flag() {
  sed -i "s/$/ ${PAUSED_FLAG}/" "${LOCKFILE}"
}

# Function to clear the paused state in the LOCKFILE
clear_paused_flag() {
  sed -i "s/ ${PAUSED_FLAG}//" "${LOCKFILE}"
}

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_INFO="$(cat "${LOCKFILE}")"
  LOCK_PID="$(echo "${LOCK_INFO}" | cut -d' ' -f1)"
  LOCK_SCRIPT="$(echo "${LOCK_INFO}" | cut -d' ' -f2)"

  if [ -n "${LOCK_PID}" ] && [ "${LOCK_SCRIPT}" = "${MAIN_SCRIPT_PATH}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    if is_paused; then
      # This means Archiver backup is paused
      if [ -z "${command}" ]; then
        echo "An Archiver backup is paused."
      elif [ "${command}" = "pause" ]; then
        echo "An Archiver backup is already paused. Use 'archiver resume' to resume."
      elif [ "${command}" = "resume" ]; then
        echo "Resuming Archiver backup."
        pkill -CONT -P "${LOCK_PID}"
        clear_paused_flag
        echo "Archiver backup resumed."
      fi
    else
      # This means Archiver backup is running
      if [ -z "${command}" ]; then
        echo "An Archiver backup is running."
      elif [ "${command}" = "pause" ]; then
        echo "Pausing Archiver backup."
        pkill -STOP -P "${LOCK_PID}"
        set_paused_flag
        echo "Archiver backup paused. Use 'archiver resume' to resume."
      elif [ "${command}" = "resume" ]; then
        echo "Archiver backup is not paused. No need to resume. Use 'archiver pause' to pause."
      fi
    fi
  else
    echo "Stale lock file detected. No running Archiver backup found with PID ${LOCK_PID}. Run 'archiver stop' to fix this."
  fi
else
  echo "Archiver backup is not running."
fi
