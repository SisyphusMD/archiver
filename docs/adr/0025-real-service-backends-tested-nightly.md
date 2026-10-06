# 25. Backends without an emulator are tested nightly

- Status: Accepted
- Date: 2026-10-06

## Context

Most storage types can be tested in CI against an emulator container (Azurite, Garage, a WebDAV or Samba server, Swift). Google Drive, OneDrive, Dropbox, B2, Storj and File Fabric have none, and Duplicacy and rclone talk to fixed endpoints, so faking them would need DNS and TLS interception.

## Options

1. Dedicated free test accounts and a nightly advisory CI job.
2. The same accounts, checked by hand before each release.
3. Config and secret wiring tests only; documented as unverified.

## Decision

Option 1.

## Consequences

- The test accounts are created together with Cody, with consistent names, a dedicated email scheme and the same settings on every service; their tokens live in Forgejo secrets, never in the repository.
- The nightly job runs a backup, a copy, a kit placement and a restore on each backend and reports failures; it does not block pull requests, since outside services flake.
- No test ever touches a production storage.
- Every pull request still tests these types' config, secrets and token handling.
