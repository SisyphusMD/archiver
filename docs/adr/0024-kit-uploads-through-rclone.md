# 24. The recovery kit uploads through rclone

- Status: Accepted
- Date: 2026-10-06

## Context

The kit is a plain file beside the backups on every storage (ADR 1 requires it to stay recoverable with stock openssl). Duplicacy cannot upload arbitrary files, so each storage type needs its own uploader. Today there are four, written natively. ADR 23 adds about a dozen more, several with OAuth or no simple HTTP API.

## Options

1. A native uploader per type.
2. A pinned rclone for the new types; the existing four stay native.
3. A pinned rclone for every type.

## Decision

Option 3.

## Consequences

- The image carries one more pinned, checksum-verified binary, tracked by Renovate.
- Every type places the kit the same way; the four native uploaders are removed.
- Credentials reach rclone through environment variables, never argv or disk.
- Where a type has file modes (local, SFTP), the kit is still made at least as readable as the storage's own files and verified, or kept out of the uploaded-state record so the next run retries.
- The kit's contents, name and encryption do not change; existing kits stay current.
