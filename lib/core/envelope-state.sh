#!/bin/bash
# The printed envelope's standing, from the two state files the envelope feature writes (no
# secrets involved, so status and healthcheck can read it without loading the configuration).
# Prints two tab-separated fields: the status line (empty when the recovery kit is not in use)
# and a healthcheck warning (empty unless a confirmed envelope is stale). $1 formats an epoch
# as an age for the status line.

ENVELOPE_STATE_SH_SOURCED=true

envelope_standing() {
  local age_fn="${1}" confirmed="" current="" cfp cepoch fp now
  [[ -f "${LOG_DIR}/.envelope-confirmed" ]] && confirmed="$(head -1 "${LOG_DIR}/.envelope-confirmed")"
  [[ -f "${LOG_DIR}/.envelope-current" ]] && current="$(head -1 "${LOG_DIR}/.envelope-current")"
  now="$(date +%s)"
  if [[ -z "${confirmed}" && -z "${current}" ]]; then
    printf '\t\n'
  elif [[ -z "${confirmed}" ]]; then
    printf "Envelope: never confirmed printed (print with 'archiver envelope', then 'archiver envelope confirm').\t\n"
  else
    read -r cfp cepoch <<<"${confirmed}"
    fp="${current%% *}"
    if [[ -n "${current}" && "${fp}" != "${cfp}" ]]; then
      printf "Envelope: OUT OF DATE, printed %s; a secret or storage on it changed.\tthe printed break-glass envelope is out of date (a secret or storage on it changed); reprint with 'archiver envelope' and confirm\n" "$("${age_fn}" "${cepoch}")"
    elif (( now - cepoch > 365 * 86400 )); then
      printf "Envelope: check due, confirmed printed %s.\tthe break-glass envelope was confirmed over a year ago; check it is still there and readable, then 'archiver envelope confirm'\n" "$("${age_fn}" "${cepoch}")"
    else
      printf "Envelope: current, confirmed printed %s (fingerprint %s).\t\n" "$("${age_fn}" "${cepoch}")" "${cfp}"
    fi
  fi
}
