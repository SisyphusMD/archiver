#!/usr/bin/env bash
# Tools a hook may need are in the image, so it need not start a helper container: btrfs
# (a read-only snapshot before a backup) and Python 3 with lmdb (compacting an LMDB index,
# as Garage's, from that snapshot).
#
#   docker run -i --rm --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/hook-tools.sh

set -uo pipefail
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

btrfs --version >/dev/null 2>&1 || die "btrfs is not in the image"
python3 - <<'PY' || die "python3 with the lmdb module is not in the image"
import lmdb, os, tempfile
d = tempfile.mkdtemp()
env = lmdb.open(d, map_size=1 << 20)
with env.begin(write=True) as txn:
    txn.put(b"k", b"v")
out = tempfile.mkdtemp()
env.copy(out, compact=True)
assert os.path.exists(os.path.join(out, "data.mdb"))
PY
echo "=== HOOK-TOOLS OK: btrfs, python3 + lmdb (compacting copy) ==="
