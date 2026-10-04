#!/bin/bash
# Serializes the configuration set in the current shell (loaded by config-loader, or answered
# in init) as a non-secret env file, one file per secret, and the key files. The variable
# surface is config-vars.sh's, which the loader shares, so a field it knows is never dropped.

CONFIG_SERIALIZE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_VARS_CORE}"

# SERVICE_DIRECTORIES is held internally as an array; emit it as the canonical colon-scalar.
service_directories_scalar() {
  local IFS=':'
  printf '%s' "${SERVICE_DIRECTORIES[*]}"
}

# Names of the currently-set effective config vars.
list_nonsecret_config_vars() { compgen -v | grep -E "${CONFIG_NONSECRET_VARS_RE}" | sort -V; }
list_secret_config_vars()    { compgen -v | grep -E "${CONFIG_SECRET_VARS_RE}"    | sort -V; }

# Write a non-secret env file (KEY=value), one file per secret under secretsdir, and the key
# files under their /run/secrets names. secretsdir files are the exact secret values.
serialize_env_and_secrets() {
  local envfile="${1}" secretsdir="${2}" v lower
  mkdir -p "${secretsdir}"
  chmod 700 "${secretsdir}"

  {
    printf 'SERVICE_DIRECTORIES=%s\n' "$(service_directories_scalar)"
    while IFS= read -r v; do
      [[ "${v}" == "SERVICE_DIRECTORIES" ]] && continue
      printf '%s=%s\n' "${v}" "${!v}"
    done < <(list_nonsecret_config_vars)
  } >"${envfile}"

  while IFS= read -r v; do
    lower="$(printf '%s' "${v}" | tr '[:upper:]' '[:lower:]')"
    printf '%s' "${!v}" >"${secretsdir}/${lower}"
    chmod 600 "${secretsdir}/${lower}"
  done < <(list_secret_config_vars)

  # Key files are secrets too; copy them under the names the entrypoint reads.
  if [[ -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ]]; then
    cp "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" "${secretsdir}/rsa_private_key"; chmod 600 "${secretsdir}/rsa_private_key"
  fi
  if [[ -f "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" ]]; then
    cp "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${secretsdir}/rsa_public_key"; chmod 644 "${secretsdir}/rsa_public_key"
  fi
  if [[ -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ]]; then
    cp "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" "${secretsdir}/ssh_private_key"; chmod 600 "${secretsdir}/ssh_private_key"
  fi
  if [[ -f "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" ]]; then
    cp "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" "${secretsdir}/ssh_public_key"; chmod 644 "${secretsdir}/ssh_public_key"
  fi
}
