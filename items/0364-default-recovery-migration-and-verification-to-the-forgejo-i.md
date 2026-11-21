# #364 Default recovery, migration and verification to the Forgejo image

- Kind: issue, state: closed, by cody, created 2026-11-18T06:33:50-08:00
- Labels: status:in-progress, type:chore; milestone: none

Use the primary image in operator instructions and cross-version verification. Keep the full GHCR mirror, same digests and compatibility with existing Go imports.

## cody, 2026-11-18T06:53:22-08:00

The local patch uses the Forgejo OCI image for migration help, recovery and cross-version checks, and keeps existing printed envelopes valid. Go race tests and vet pass. The historical image digests resolve from Forgejo. The storage examples now select a published image that contains rclone. The public registry endpoint currently returns HTTP 500. Landing waits for an anonymous pull through the public endpoint.

## cody, 2026-11-20T08:51:56-08:00

Done: PR #369 merged at 5a59992 (all required contexts green on 3374329, incl. black-box cross-version suite pulling baselines from Forgejo), GitHub and NAS mirrors match, branch and worktree removed. Review fixes: release script now replaces whole prerelease tags; legacy guide pins floating 0.11.
