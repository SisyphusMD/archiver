# #49 feat: least-privilege restore capabilities (warn, do not block, when CHOWN/FOWNER missing)

- Kind: pull, state: closed, by cody, created 2026-07-07T18:46:28-07:00
- Labels: none; milestone: none
- refs/pull/49/head -> main, merged: True

Phase 4: least-privilege backup/restore capability profiles.

`CHOWN` and `FOWNER` are only used by restore (to recreate original UID/GID and set modes on other-UID files). So a backup-only deployment can run with just `DAC_OVERRIDE` and add `CHOWN` + `FOWNER` only when restoring.

To keep that safe (a bare cap split would quietly reintroduce the old silent-root-ownership bug), restore now **warns without ever blocking**: if it would preserve ownership but the container lacks `CHOWN`/`FOWNER`, it logs a clear warning that files will land root-owned and how to fix it. The restore always proceeds (the data is what matters in DR); set `IGNORE_OWNERSHIP=1` to restore without ownership on purpose. The capability check reads `/proc/self/status` `CapEff` (no side effects, no false positives) and short-circuits when `-ignore-owner` is in effect.

Docs: README cap table + a "backup-only least privilege" note + both cap_add examples updated; compose.yaml notes CHOWN/FOWNER are restore-only and droppable.

Verified locally: warns under backup-only caps, silent under full caps, silent with -ignore-owner; the normal restore round-trip is unaffected (no spurious warning); bats 36/36. New `tests/integration/restore-caps.sh` runs under both cap sets in CI.

No version bump; final piece of the env-native rework, accumulating under `[Unreleased]` toward `0.9.0`.
