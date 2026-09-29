#!/usr/bin/env bash
# Run the black-box suite against ARCHIVER_IMAGE (default archiver:dev).
#
#   ARCHIVER_IMAGE=archiver:dev tests/e2e/run.sh -run CrossVersion -v
#
# Archiver containers are started as siblings of the runner, so every path the tests
# bind-mount must mean the same thing inside the runner and on the Docker host: the work
# tree lives at the same absolute path in both.
set -euo pipefail

cd "$(dirname "$0")/../.."
WORK=/tmp/archiver-e2e
RUNNER=archiver-e2e-runner:local

docker build -q -f tests/e2e/Dockerfile -t "$RUNNER" . >/dev/null
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$WORK:$WORK" \
  -e ARCHIVER_E2E_WORK="$WORK" \
  -e ARCHIVER_IMAGE="${ARCHIVER_IMAGE:-archiver:dev}" \
  ${ARCHIVER_BASELINE_IMAGE:+-e ARCHIVER_BASELINE_IMAGE="$ARCHIVER_BASELINE_IMAGE"} \
  "$RUNNER" "$@"
