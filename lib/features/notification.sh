#!/bin/bash

NOTIFICATION_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
source_if_not_sourced "${CONFIG_LOADER_CORE}"

send_pushover_notification() {
  local title="${1}"
  local message="${2}"
  local exit_status

  # The credentials reach curl as a config file on stdin (-K -), never as arguments, which
  # any process can read from /proc. Inside its double quotes \ and " must be escaped.
  local token="${PUSHOVER_API_TOKEN//\\/\\\\}" user="${PUSHOVER_USER_KEY//\\/\\\\}"
  token="${token//\"/\\\"}"
  user="${user//\"/\\\"}"
  printf 'form-string = "token=%s"\nform-string = "user=%s"\n' "${token}" "${user}" | \
    curl -s -K - \
      --form-string "title=${title}" \
      --form-string "message=${message}" \
      https://api.pushover.net/1/messages.json | log_output
  exit_status="${PIPESTATUS[1]}"

  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Failed to send pushover notification. Check Pushover variables in the secrets file."
  else
    log_message "INFO" "Pushover notification sent successfully."
  fi
}

notify() {
  local title="${1}"
  local message="${2}"
  local timestamp
  local formatted_message

  [ -z "${NOTIFICATION_SERVICE}" ] && return 0

  # Prepend hostname and timestamp to message
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  formatted_message="[${HOSTNAME}] [${timestamp}] ${message}"

  if [ "$(echo "${NOTIFICATION_SERVICE}" | tr '[:upper:]' '[:lower:]')" == "pushover" ]; then
    send_pushover_notification "${title}" "${formatted_message}"
  fi
}
