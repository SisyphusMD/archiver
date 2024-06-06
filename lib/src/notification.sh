#!/bin/bash

send_pushover_notification() {
  local title
  local message
  local exit_status

  title="${1}"
  message="${2}"

  # Sending the Pushover notification
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
  local title
  local message

  title="${1}"
  message="${2}"

  if [ "$(echo "${NOTIFICATION_SERVICE}" | tr '[:upper:]' '[:lower:]')" == "pushover" ]; then
    send_pushover_notification "${title}" "${message}"
  fi
}