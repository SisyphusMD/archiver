# Notifies user of any errors, as well as successful completion of the backup
# Parameters:
#   1. Title: The title of the notification.
#   2. Message: The body of the notification.
# Output:
#   Call the notification API with the provided parameters.
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
    https://api.pushover.net/1/messages.json | log_output "curl"
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