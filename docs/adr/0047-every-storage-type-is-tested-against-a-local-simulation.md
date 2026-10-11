# 47. Every storage type is tested against a local simulation

- Status: Accepted
- Date: 2026-10-10
- Supersedes: 25, 27

## Context

ADRs 25 and 27 test Google Drive, OneDrive, Dropbox, B2, Storj and File Fabric
against dedicated live accounts, weekly and before each release. That makes
tests depend on outside services, accounts and payment terms (Storj and File
Fabric need a card or trial, OneDrive's signup is blocked), and they cannot run
offline or consistently.

## Options

1. Keep live test accounts for the types without an emulator (ADRs 25, 27).
2. Simulate every type locally: purpose-built fakes for B2, Google Drive,
   OneDrive, Dropbox and File Fabric behind their real hostnames, and a local
   Storj network (storj-up).
3. Simulate where an emulator exists; test the rest's wiring only.

## Decision

Option 2.

## Consequences

- Every storage type runs its full round trip (primary and copy, kit, restore,
  maintenance) in normal CI, offline, with no accounts or secrets.
- The real-service job, its release gate and `skip_real_backends` are removed,
  with the NIGHTLY_* secrets.
- A provider changing its API is no longer caught by our tests.
- The live test accounts are no longer used; closing them is a separate step.
