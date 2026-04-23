#!/bin/bash
# Main CLI entrypoint for Archiver commands

ARCHIVER_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${REQUIRE_CONTAINER_CORE}"

usage() {
  echo "Usage: $0 {start|stop|pause|resume|restart|logs|status|bundle|restore|auto-restore|snapshot-exists|healthcheck|help} [logs|prune|retain]"
  echo "Note:"
  echo "  stop|pause|logs|status|restore|auto-restore|snapshot-exists|healthcheck|help cannot have further arguments."
  echo "  start may be used in combination with logs and prune|retain."
  echo "  resume|restart may be used in combination with logs."
  echo "  bundle requires a subcommand: export or import"
  echo "  auto-restore and snapshot-exists are non-interactive and driven by environment variables."
  exit 1
}

logs="false"
args=()

if [[ $# -lt 1 ]]; then
  echo "No arguments provided."
  usage
fi

command="${1}"
shift

start_archiver() {
  # Parse optional flags: logs, prune, retain
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

  # setsid + nohup prevents issues with exported env vars when log view is closed
  setsid nohup "${MAIN_SCRIPT}" "${args[@]}" &>/dev/null &
  echo "Archiver main script called in the background."
}

# Route commands to their respective scripts
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
    if [[ "${command}" == "restart" ]]; then
      "${STOP_SCRIPT}"
    fi
    start_archiver "${@}"
    ;;
  stop)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    "${STOP_SCRIPT}"
    ;;
  pause)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    "${PAUSE_SCRIPT}"
    ;;
  resume)
    if [[ $# -gt 1 || ( $# -eq 1 && "${1}" != "logs" ) ]]; then
      echo "'${*:1}' is not valid for 'archiver ${command}'."
      usage
    fi
    if [[ -n "${1}" ]]; then
      logs="true"
    fi
    "${RESUME_SCRIPT}"
    ;;
  logs)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    "${LOGS_SCRIPT}"
    ;;
  status)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    "${STATUS_SCRIPT}"
    ;;
  restore)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    "${RESTORE_SCRIPT}"
    ;;
  auto-restore)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    exec "${AUTO_RESTORE_SCRIPT}"
    ;;
  snapshot-exists)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    exec "${SNAPSHOT_EXISTS_SCRIPT}"
    ;;
  healthcheck)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    exec "${HEALTHCHECK_SCRIPT}"
    ;;
  backup)
    if [[ $# -gt 0 ]]; then
      case "${1}" in
        prune|retain)
          if [[ $# -gt 1 ]]; then
            echo "'${*:2}' is not valid for 'archiver ${command} ${1}'."
            usage
          fi
          args=("${1}")
          ;;
        *)
          echo "'${*:1}' is not valid for 'archiver ${command}'."
          usage
          ;;
      esac
    fi
    "${MAIN_SCRIPT}" "${args[@]}"
    exit $?
    ;;
  bundle)
    if [[ $# -ne 1 ]]; then
      echo "'bundle' requires exactly one subcommand: export or import"
      usage
    fi
    subcommand="${1}"
    if [[ "${subcommand}" == "export" ]]; then
      "${BUNDLE_EXPORT_SCRIPT}"
    elif [[ "${subcommand}" == "import" ]]; then
      "${BUNDLE_IMPORT_SCRIPT}"
    else
      echo "Unknown bundle subcommand: ${subcommand}"
      echo "Valid subcommands: export, import"
      usage
    fi
    ;;
  help)
    usage
    ;;
  *)
    echo "Unknown command: ${command}"
    usage
    ;;
esac

if [[ "${logs}" == "true" ]]; then
  "${LOGS_SCRIPT}"
fi

exit 0
