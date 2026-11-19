# #364 Default recovery, migration and verification to the Forgejo image

- Kind: issue, state: open, by cody, created 2026-11-18T06:33:50-08:00
- Labels: status:blocked, type:chore; milestone: none

Use the primary image in operator instructions and cross-version verification. Keep the full GHCR mirror, same digests and compatibility with existing Go imports.

## cody, 2026-11-18T06:53:22-08:00

The local patch uses the Forgejo OCI image for migration help, recovery and cross-version checks, and keeps existing printed envelopes valid. Go race tests and vet pass. The historical image digests resolve from Forgejo. The storage examples now select a published image that contains rclone. The public registry endpoint currently returns HTTP 500. Landing waits for an anonymous pull through the public endpoint.
