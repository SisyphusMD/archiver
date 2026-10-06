# 26. Duplicacy is built from pinned source with a Dropbox patch

- Status: Accepted
- Date: 2026-10-06
- Supersedes: ADR 7's use of the released binary (the CLI still runs as a child process)

## Context

ADR 23 adds every Duplicacy backend. Duplicacy 3.2.5, its latest release, refreshes Dropbox tokens only through duplicacy.com, which keeps its app's secret; unlike OneDrive, it offers no way to use your own app. A Dropbox backup would then stop whenever that site does, a third-party dependency of both backups and recovery. Duplicacy's license permits modification and redistribution, with derivative works under the same terms as the original.

## Options

1. The released binary; Dropbox depends on duplicacy.com, flagged.
2. The released binary, with duplicacy.com redirected to a local refresher inside the container.
3. Build the pinned 3.2.5 source with a one-line patch so Dropbox refreshes directly against Dropbox with the user's own app; offer the change upstream.

## Decision

Option 3.

## Consequences

- The image builds Duplicacy from a pinned, checksum-verified source tag plus one reviewed patch; the CLI still runs as a child process, as ADR 7 decided.
- No backend depends on duplicacy.com: Google Drive and OneDrive also use the user's own app or a service account.
- Every other behavior is the release's; the cross-version tests run against the patched build.
- A Duplicacy upgrade means re-applying or dropping the patch; it is dropped once upstream supports a custom Dropbox app.
- The upstream change goes through Cody's contribution review before it is sent.
