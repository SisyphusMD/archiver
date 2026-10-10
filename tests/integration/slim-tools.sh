#!/usr/bin/env bash
# The slim image (ADR 41) holds everything Archiver itself runs and none of the tools only
# hooks use; the full image holds both. Run inside the image under test with SLIM=1 or 0.
#
#   docker run -i --rm -e SLIM=1 --entrypoint bash archiver:slim -s < tests/integration/slim-tools.sh

set -uo pipefail
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

for t in archiver duplicacy rclone tini openssl ssh sftp ssh-keyscan ssh-keygen curl qrencode tar flock bash; do
  command -v "$t" >/dev/null || die "$t is missing: Archiver runs it"
done
hook_tools=(docker python3 sqlite3 systemctl zfs btrfs vim nano ping pgrep)
for t in "${hook_tools[@]}"; do
  if [ "${SLIM:?set SLIM=1 or 0}" = 1 ]; then
    command -v "$t" >/dev/null && die "$t is in the slim image: only hooks use it"
  else
    command -v "$t" >/dev/null || die "$t is missing from the full image: hooks use it"
  fi
done
python3 -c 'import lmdb' 2>/dev/null || [ "$SLIM" = 1 ] || die "python3-lmdb is missing from the full image"
echo "PASS: image tools (SLIM=$SLIM)"
