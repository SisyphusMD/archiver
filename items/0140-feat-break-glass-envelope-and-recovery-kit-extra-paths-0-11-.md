# #140 feat: break-glass envelope and recovery-kit extra paths (0.11.5 candidate)

- Kind: pull, state: closed, by cody, created 2026-10-04T17:19:13-07:00
- Labels: none; milestone: none
- refs/pull/140/head -> release/0.11, merged: True

Backport of main #139 (Cody 2026-10-04: ship on both lines, a feature in a 0.11 patch by exception to ADR 9). Same envelope and kit code; 0.11 has no Go, so lib/core/envelope-state.sh gives bash status/healthcheck the same envelope lines. Full 0.11 suite passes, envelope test included.
