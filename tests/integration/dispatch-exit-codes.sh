#!/usr/bin/env bash
# A command that fails exits non-zero through the `archiver` dispatcher too, not only when
# its script is run directly: a failed restore or migrate never reports success.
#
#   docker run -i --rm --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/dispatch-exit-codes.sh

set -uo pipefail

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "restore with no configuration fails"
archiver restore </dev/null >/tmp/out 2>&1 && { cat /tmp/out; die "a failed restore exited 0"; }

log "migrate into an unwritable directory fails"
archiver migrate /proc/archiver-migrate >/tmp/out 2>&1 && { cat /tmp/out; die "a failed migrate exited 0"; }

echo "=== DISPATCH-EXIT-CODES OK ==="
