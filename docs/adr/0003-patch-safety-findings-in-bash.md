# 3. Patch safety findings in bash before v1

- Status: Accepted
- Date: 2026-09-26

## Context

The Phase 1 audit found defects in the current release that affect data safety or service availability. v1 is months away, and nas and vps back up nightly on the bash release.

## Options

1. Patch the safety findings in bash now and leave user-experience items to v1.
2. Patch everything the audit found in bash now.
3. Patch nothing and fold every finding into v1.

## Decision

Option 1. A bash patch release fixes:

- the post-backup hook being skipped when a backup fails or is stopped mid-backup
- storage secrets written in plaintext to `<service>/.duplicacy/preferences`, and exposed on argv, by `duplicacy set`
- pre-backup hook failures being ignored, which produces a false "Completed successfully"
- `SERVICE_DIRECTORIES` entries that match nothing, or contain a space, being skipped without any report
- the README Kubernetes CronJob example lacking a stable hostname, which gives every run a new snapshot ID
- binary downloads in the Dockerfile lacking checksum verification

Each behavioral fix starts with an integration test that fails on the current image. The alert flood, the stop error count, and Ctrl+C in `archiver logs` wait for v1.

## Consequences

- Runs that used to pass while silently skipping data or hooks will now exit non-zero. That is the intended effect.
- nas and vps pick up the fixes through their normal image bump.
- The new tests guard the bash releases. Under ADR 2 they count as incidental, so v1 is not held to them unless Cody marks one as contract.
