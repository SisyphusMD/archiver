#!/bin/bash

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
source_if_not_sourced "${CONFIG_CORE}"

NOTIFICATION_SH_SOURCED=true

send_pushover_notification() {
  local title="${1}"
  local message="${2}"
  local exit_status

  curl -s \
    --form-string "token=${PUSHOVER_API_TOKEN}" \
    --form-string "user=${PUSHOVER_USER_KEY}" \
    --form-string "title=${title}" \
    --form-string "message=${message}" \
    https://api.pushover.net/1/messages.json | log_output
  exit_status="${PIPESTATUS[0]}"

  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Failed to send pushover notification. Check Pushover variables in the secrets file."
  fi
  log_message "INFO" "Pushover notification sent successfully."
}

notify() {
  local title="${1}"
  local message="${2}"

  [ -z "${NOTIFICATION_SERVICE}" ] && return 0

  if [ "$(echo "${NOTIFICATION_SERVICE}" | tr '[:upper:]' '[:lower:]')" == "pushover" ]; then
    send_pushover_notification "${title}" "${message}"
  fi
}
