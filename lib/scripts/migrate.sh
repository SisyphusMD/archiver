#!/bin/bash
# `archiver migrate [OUTPUT_DIR]` — spread the effective configuration into env-native
# materials: a non-secret .env file plus a one-file-per-secret directory (including the keys),
# ready to load into a Docker Compose environment, a Kubernetes ConfigMap + Secret, Docker
# secrets, or openbao. Reads the effective config via config-loader, so it works whether the
# container is running from a bundle or already env-native.

MIGRATE_SH_SOURCED=true

set -e
# Everything written below is secret material; never let it exist world-readable, even briefly.
umask 077

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${CONFIG_LOADER_CORE}"      # loads the effective configuration
source_if_not_sourced "${CONFIG_SERIALIZE_CORE}"

OUTPUT_DIR="${1:-/opt/archiver/migrate}"
ENV_FILE="${OUTPUT_DIR}/archiver.env"
SECRETS_OUT="${OUTPUT_DIR}/secrets"

if ! mkdir -p "${OUTPUT_DIR}"; then
  echo "ERROR: could not create output directory ${OUTPUT_DIR}" >&2
  exit 1
fi

serialize_env_and_secrets "${ENV_FILE}" "${SECRETS_OUT}"

echo "Migrated the effective configuration to ${OUTPUT_DIR}:"
echo "  ${ENV_FILE}"
echo "      non-secret settings as KEY=value (a Compose 'environment:' block or a k8s ConfigMap)"
echo "  ${SECRETS_OUT}/"
echo "      one file per secret plus the keys (Docker secrets, a k8s Secret, or openbao)"
echo
echo "SECURITY: these files hold your secrets in PLAINTEXT. Move them into your secret store"
echo "(Docker secrets, a Kubernetes Secret, openbao) and delete this directory."
echo
echo "Next steps:"
echo "  1. Load ${ENV_FILE##*/} as environment variables (ConfigMap / compose environment)."
echo "  2. Load the ${SECRETS_OUT##*/}/ files as secrets mounted under /run/secrets."
echo "  3. Start the container with no bundle. See the README 'Configuration Sources' section."
