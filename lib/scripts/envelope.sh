#!/bin/bash
# `archiver envelope [DIR]` writes the break-glass envelope (HTML and PDF) for printing;
# `archiver envelope confirm` records that the current page has been printed, so status can
# say when the paper goes out of date.

# Distinct guard name: the envelope FEATURE shares this basename (see recovery-kit.sh).
ENVELOPE_SCRIPT_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_LOADER_CORE}"
source_if_not_sourced "${ENVELOPE_FEATURE}"

if ! recovery_kit_configured; then
  echo "The envelope carries the recovery kit's password, and the recovery kit is not configured. Provide the recovery password at ${SECRETS_DIR}/recovery_password (or point RECOVERY_PASSWORD_FILE at it)." >&2
  exit 1
fi

count_storage_targets
verify_target_settings
check_required_secrets

if [[ "${1:-}" == "confirm" ]]; then
  rc=0; fp="$(envelope_confirm)" || rc=$?
  case ${rc} in
    0) ;;
    2) echo "No envelope has been written yet: run 'archiver envelope', print it, then confirm." >&2; exit 1 ;;
    3) echo "The configuration changed after the envelope was written, so the printed page is already out of date. Run 'archiver envelope' again, print the new page, then confirm." >&2; exit 1 ;;
    *) echo "Could not record the envelope as printed." >&2; exit 1 ;;
  esac
  echo "Recorded envelope ${fp} as printed. 'archiver status' will say when it goes out of date."
  exit 0
fi

dir="${1:-${ARCHIVER_DIR}/envelope}"
fp="$(envelope_write "${dir}")" || { echo "Could not write the envelope to ${dir}." >&2; exit 1; }
host="$(recovery_kit_host)"
echo "Wrote the break-glass envelope (fingerprint ${fp}):"
[[ -f "${dir}/envelope-${host}.pdf" ]] && echo "  ${dir}/envelope-${host}.pdf"
echo "  ${dir}/envelope-${host}.html   (the same page, for printing from a browser)"
cat <<MSG

They hold your recovery password and storage credentials in PLAINTEXT. Print one, then:
  1. archiver envelope confirm
  2. delete both files (and any copies, e.g. after 'docker cp').
MSG
