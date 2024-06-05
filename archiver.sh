#!/bin/bash

# Record the start time before calling main.sh
START_TIME="$(date +%s)"

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

usage() {
  echo "Usage: $0 {start|stop|pause|resume|restart|logs|status|setup|uninstall|restore|help} [logs|prune|retain]"
  echo "Note:"
  echo "  stop|pause|logs|status|setup|uninstall|restore|help cannot have further arguments."
  echo "  start may be used in combination with logs and prune|retain."
  echo "  resume|restart may be used in combination with logs."
  exit 1
}

# Work through arguments

logs="false"
args=()

if [[ $# -lt 1 ]]; then
  echo "No arguments provided."
  usage
fi

command="${1}"
shift

case "${command}" in
  start)
    if [[ $# -gt 0 ]]; then
      case "${1}" in
        logs)
          if [[ $# -gt 2 || ( $# -eq 2 && ! ( "${2}" == "prune" || "${2}" == "retain" ) ) ]]; then
            echo "'${2}' is not a valid argument for 'archiver start logs'."
            usage
          fi
          ;;
        prune|retain)
          if [[ $# -gt 2 || ( $# -eq 2 && "${2}" != "logs" ) ]]; then
            echo "'${2}' is not a valid argument for 'archiver start ${1}'."
            usage
          fi
          ;;
        *)
          echo "'${1}' is not a valid argument for 'archiver start'."
          usage
          ;;
      esac
    fi
    ;;
  resume|restart)
    if [[ $# -gt 1 || ( $# -eq 1 && "${1}" != "logs" ) ]]; then
      echo "'${1}' is not a valid argument for '${command}'."
      usage
    fi
    ;;
  stop|pause|logs|status|setup|uninstall|restore|help)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    ;;
  *)
    echo "Unknown command: ${command}"
    usage
    ;;
esac

# Define the path to the Archiver script
ARCHIVER_SCRIPT="$(realpath "${0}")"
ARCHIVER_DIR="$(dirname "${ARCHIVER_SCRIPT}")"
# Define mod directory
MOD_DIR="${ARCHIVER_DIR}/lib/mod"
# Define paths to various scripts
MAIN_SCRIPT="${MOD_DIR}/main.sh"
LOGS_SCRIPT="${MOD_DIR}/logs.sh"
STOP_SCRIPT="${MOD_DIR}/stop.sh"
SETUP_SCRIPT="${MOD_DIR}/setup.sh"
STATUS_SCRIPT="${MOD_DIR}/status.sh"
RESTORE_SCRIPT="${MOD_DIR}/restore.sh"

# Archiver Start Logic
if [[ "${command}" == "start" ]]; then
  if [[ -n "${1}" ]]; then
    if [[ "${1}" == "logs" ]]; then
      logs="true"
      if [[ -n "${2}" ]]; then
        args=("${2}")
      fi
    elif [[ "${1}" == "prune" ]] || [[ "${1}" == "retain" ]]; then
      args=("${1}")
      if [[ -n "${2}" ]]; then
        logs="true"
      fi
    fi
  fi
  # Start Archiver in a new session using setsid
  setsid nohup "${MAIN_SCRIPT}" "${args[@]}" &>/dev/null & # setsid + nohup was required to fix bug related to duplicacy exported env vars when user ran script with --view-logs, then closed that running log view
  echo "Archiver main script called in the background."
fi

# Archiver stop logic
if [[ "${command}" == "stop" ]]; then
  "${STOP_SCRIPT}"
fi

# Archiver setup logic
if [[ "${command}" == "setup" ]]; then
  "${SETUP_SCRIPT}"
fi

# Archiver setup logic
if [[ "${command}" == "restore" ]]; then
  "${RESTORE_SCRIPT}"
fi

# Archiver status logic
if [[ "${command}" == "status" ]]; then
  "${STATUS_SCRIPT}"
fi

# Archiver logs logic
if [[ "${command}" == "logs" ]] || [[ "${logs}" == "true" ]]; then
  "${LOGS_SCRIPT}" --start-time "${START_TIME}"
fi

# Others to write
# Still need to write functions for pause|resume|restart|uninstall|help

exit 0
