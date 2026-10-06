# 23. Support every Duplicacy backend

- Status: Accepted
- Date: 2026-10-06

## Context

Archiver supports four of the storage types Duplicacy 3.2.5 can use: local, SFTP, B2 and S3. Duplicacy's source also supports S3 variants (path-style, plain HTTP, Wasabi), B2 at a custom endpoint, Azure Blob, Google Cloud Storage, Google Drive, OneDrive (personal and business), Dropbox, OpenStack Swift, WebDAV, SMB, Storj and File Fabric. Amazon Cloud Drive and hubiC are in the code, but both services have shut down.

## Options

1. A chosen subset, by effort and testability.
2. Every backend Duplicacy supports that still works.

## Decision

Option 2.

## Consequences

- Each type gets config validation, file-only secrets, a recovery-kit upload, an envelope block, docs and a test.
- Amazon Cloud Drive and hubiC are left out: no service exists to use them with.
- OneDrive rewrites its token file, so its token lives at a writable path rather than in the read-only secrets.
- New types declare their default check interval (ADR 17).
- Storage names and URLs of the existing four types do not change (ADR 1).
