#!/bin/bash

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${REQUIRE_DOCKER_CORE}"

usage() {
  echo "Usage: $0 {start|stop|pause|resume|restart|logs|status|bundle|restore|healthcheck|help} [logs|prune|retain]"
  echo "Note:"
  echo "  stop|pause|logs|status|restore|healthcheck|help cannot have further arguments."
  echo "  start may be used in combination with logs and prune|retain."
  echo "  resume|restart may be used in combination with logs."
  echo "  bundle requires a subcommand: export or import"
  exit 1
}

# Initialize variables for parsing command arguments
logs="false"  # Flag to track if logs should be displayed
args=()       # Array to hold additional command arguments (prune/retain)

if [[ $# -lt 1 ]]; then
  echo "No arguments provided."
  usage
fi

command="${1}"
shift

case "${command}" in
  start|restart)
    if [[ $# -gt 0 ]]; then
      case "${1}" in
        logs)
          if [[ $# -gt 2 || ( $# -eq 2 && ! ( "${2}" == "prune" || "${2}" == "retain" ) ) ]]; then
            echo "'${*:2}' is not valid for 'archiver ${command} ${1}'."
            usage
          fi
          ;;
        prune|retain)
          if [[ $# -gt 2 || ( $# -eq 2 && "${2}" != "logs" ) ]]; then
            echo "'${*:2}' is not valid for 'archiver ${command} ${1}'."
            usage
          fi
          ;;
        *)
          echo "'${*:1}' is not valid for 'archiver ${command}'."
          usage
          ;;
      esac
    fi
    ;;
  resume)
    if [[ $# -gt 1 || ( $# -eq 1 && "${1}" != "logs" ) ]]; then
      echo "'${*:1}' is not valid for 'archiver ${command}'."
      usage
    fi
    ;;
  stop|pause|logs|status|restore|healthcheck|help)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    ;;
  bundle)
    if [[ $# -ne 1 ]]; then
      echo "'bundle' requires exactly one subcommand: export or import"
      usage
    fi
    if [[ "${1}" != "export" && "${1}" != "import" ]]; then
      echo "Unknown bundle subcommand: ${1}"
      echo "Valid subcommands: export, import"
      usage
    fi
    ;;
  *)
    echo "Unknown command: ${command}"
    usage
    ;;
esac

source "/opt/archiver/lib/core/common.sh"

start_archiver() {
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
}

# Archiver Start Logic
if [[ "${command}" == "start" ]]; then
  start_archiver "${@}"
fi

# Archiver stop logic
if [[ "${command}" == "stop" ]]; then
  "${STOP_SCRIPT}"
fi

# Archiver restore logic
if [[ "${command}" == "restore" ]]; then
  "${RESTORE_SCRIPT}"
fi

# Archiver status logic
if [[ "${command}" == "status" ]]; then
  "${STATUS_SCRIPT}"
fi

# Archiver pause logic
if [[ "${command}" == "pause" ]]; then
  "${PAUSE_SCRIPT}"
fi

# Archiver resume logic
if [[ "${command}" == "resume" ]]; then
  if [[ -n "${1}" ]]; then
    logs="true"
    START_TIME=0
  fi
  "${RESUME_SCRIPT}"
fi

# Archiver restart logic
if [[ "${command}" == "restart" ]]; then
  # First stop
  "${STOP_SCRIPT}"

  # Then start again with arguments as needed
  start_archiver "${@}"
fi

# Archiver logs logic
if [[ "${command}" == "logs" ]] || [[ "${logs}" == "true" ]]; then
  "${LOGS_SCRIPT}"
fi

# Archiver help logic
if [[ "${command}" == "help" ]]; then
  usage
fi

# Archiver bundle logic
if [[ "${command}" == "bundle" ]]; then
  subcommand="${1}"
  if [[ "${subcommand}" == "export" ]]; then
    "${BUNDLE_EXPORT_SCRIPT}"
  elif [[ "${subcommand}" == "import" ]]; then
    "${BUNDLE_IMPORT_SCRIPT}"
  fi
fi

# Archiver healthcheck logic
if [[ "${command}" == "healthcheck" ]]; then
  "${HEALTHCHECK_SCRIPT}"
fi

exit 0
