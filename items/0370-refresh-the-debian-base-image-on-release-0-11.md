# #370 Refresh the Debian base image on release/0.11

- Kind: issue, state: closed, by cody, created 2026-11-20T17:26:25-08:00
- Labels: status:in-progress, type:chore; milestone: none

The 0.11 maintenance line still builds on debian:trixie-20260112-slim (January 2026), ten months of Debian security updates behind. Refresh it to the trixie-20261104-slim digest main already uses, as a maintenance patch on release/0.11.

This is also the real maintenance change that project-rules#74 and #86 wait on to prove the trusted-main release path for a maintenance line, and that clears #357/#356.

Scope: the base image digest only. The Docker CLI (29.6.2) stays put on this line: a minor bump needs hand-verified per-arch checksums and is not needed for the refresh. Releasing v0.11.8 waits on Cody's explicit yes.

## cody, 2026-11-20T17:29:19-08:00

In progress (Project-rules rollout follow-ups session): PR #371 into release/0.11 refreshes the Debian base digest to the one main uses and adds a Security changelog line. qx review taken: changelog wording fixed, non-deps commit subject so release notes do not duplicate it. Next: CI green, merge into release/0.11, then v0.11.8 only on Cody's explicit yes.

## cody, 2026-11-20T17:35:45-08:00

Done: #371 merged into release/0.11 at 8fb87e1 (ancestry verified, all CI green, branch removed). No tag or release was cut. v0.11.8 waits on Cody's explicit yes; the release proof is tracked in project-rules#74 and #86.
