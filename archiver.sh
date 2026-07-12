#!/bin/bash
# Main CLI entrypoint for Archiver commands

# shellcheck disable=SC2034  # sourced by external scripts that gate re-source on this sentinel
ARCHIVER_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${REQUIRE_CONTAINER_CORE}"
# Safe here only because lockfile.sh is dependency-free (see its header): this dispatcher
# backgrounds main.sh, so nothing it sources may mutate the environment the child inherits.
source_if_not_sourced "${LOCKFILE_CORE}"

usage() {
  echo "Usage: $0 {backup|maintenance|stop|pause|resume|logs|status|bundle|migrate|recovery-kit|restore|auto-restore|auto-restore-all|snapshot-exists|healthcheck|help}"
  echo "Note:"
  echo "  backup runs the backup pipeline (hooks -> backup -> copies); add --detach to run it in the background."
  echo "  maintenance runs per-storage check + prune now (normally scheduled via MAINTENANCE_SCHEDULE); 'maintenance exhaustive' forces the full-listing prune."
  echo "  stop takes an optional target (backup|maintenance|all, default all) and --immediate."
  echo "  resume may be used in combination with logs."
  echo "  bundle requires a subcommand: export or import"
  echo "  migrate takes an optional OUTPUT_DIR (default /opt/archiver/migrate): writes the effective config as an env file + secret files."
  echo "  recovery-kit uploads the encrypted recovery kit to every storage target; 'recovery-kit force' re-uploads even if unchanged."
  echo "  pause|logs|status|restore|auto-restore|auto-restore-all|snapshot-exists|healthcheck|help cannot have further arguments."
  echo "  auto-restore and snapshot-exists are non-interactive and driven by environment variables."
  exit 1
}

logs="false"

if [[ $# -lt 1 ]]; then
  echo "No arguments provided."
  usage
fi

command="${1}"
shift

# Route commands to their respective scripts
case "${command}" in
  start|restart)
    echo "'${command}' was removed. Use 'archiver backup --detach' to run a backup in the background" >&2
    echo "(and 'archiver stop backup' first if you were restarting). Pruning moved to 'archiver maintenance'." >&2
    exit 1
    ;;
  stop)
    targets=0
    for a in "${@}"; do
      case "${a}" in
        backup|maintenance|all) targets=$((targets + 1)) ;;
        --immediate) ;;
        *)
          echo "'${a}' is not valid for 'archiver stop' (allowed: backup, maintenance, all, --immediate)."
          usage
          ;;
      esac
    done
    # More than one target is ambiguous and would silently take only the last (backup/maintenance
    # short-circuit); require exactly one (or none, defaulting to all).
    if [[ "${targets}" -gt 1 ]]; then
      echo "'archiver stop' takes at most one target (backup|maintenance|all)."
      usage
    fi
    "${STOP_SCRIPT}" "${@}"
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
  auto-restore-all)
    if [[ $# -gt 0 ]]; then
      echo "'${command}' cannot have further arguments."
      usage
    fi
    exec "${AUTO_RESTORE_ALL_SCRIPT}"
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
    detach=false
    for a in "${@}"; do
      case "${a}" in
        --detach|-d) detach=true ;;
        prune|retain)
          echo "'${a}' was removed: backups no longer prune. Pruning runs on MAINTENANCE_SCHEDULE or via 'archiver maintenance'." >&2
          exit 1
          ;;
        *)
          echo "'${a}' is not valid for 'archiver backup' (allowed: --detach)."
          usage
          ;;
      esac
    done
    if [[ "${detach}" == "true" ]]; then
      # Refuse a live lock up front, visibly: the backgrounded main.sh discards its
      # output, so its own refusal would be invisible and detach would falsely report
      # success. A STALE lock (dead PID) is not a refusal — acquire_lock cleans it up.
      if is_lock_valid; then
        echo "A backup is already running (PID $(get_lock_pid)). Not starting another." >&2
        exit 1
      fi
      # setsid + nohup detaches from this exec session entirely.
      setsid nohup "${MAIN_SCRIPT}" &>/dev/null &
      echo "Backup started in the background (follow with 'archiver logs')."
      exit 0
    fi
    "${MAIN_SCRIPT}"
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
  migrate)
    if [[ $# -gt 1 ]]; then
      echo "'migrate' takes at most one argument: the output directory."
      usage
    fi
    "${MIGRATE_SCRIPT}" "${@}"
    ;;
  recovery-kit)
    if [[ $# -gt 1 || ( $# -eq 1 && "${1}" != "force" ) ]]; then
      echo "'recovery-kit' takes at most one argument: force."
      usage
    fi
    "${RECOVERY_KIT_SCRIPT}" "${@}"
    exit $?
    ;;
  maintenance)
    if [[ $# -gt 1 || ( $# -eq 1 && "${1}" != "exhaustive" ) ]]; then
      echo "'maintenance' takes at most one argument: exhaustive."
      usage
    fi
    "${MAINTENANCE_SCRIPT}" "${@}"
    exit $?
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
