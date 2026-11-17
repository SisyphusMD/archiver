# #173 feat: hook tools in the image (0.11.7 candidate)

- Kind: pull, state: closed, by cody, created 2026-10-08T04:35:10-07:00
- Labels: none; milestone: none
- refs/pull/173/head -> release/0.11, merged: True

Cody 2026-10-08 (via the DR session, confirmed here): add the tools the nas Garage hook needs instead of a helper container: `btrfs-progs`, `python3`, `python3-lmdb`. The hook snapshots the repo share read-only with btrfs right before the Garage backup and compacts Garage's LMDB index from that snapshot, so the index and the blocks come from one instant.

- Integration test `hook-tools.sh` (btrfs present; Python lmdb makes a compacting copy) was red on the 0.11.x image and is green here; the round-trip still passes. The image grows from 1.02 to 1.03 GB.
- `SYS_ADMIN` is documented as a deployment opt-in for hooks that snapshot btrfs (README warning, commented line in compose.yaml), per Cody. Archiver itself still needs only DAC_OVERRIDE (+ CHOWN/FOWNER for ownership on restore).

Ported to main in a separate PR.
