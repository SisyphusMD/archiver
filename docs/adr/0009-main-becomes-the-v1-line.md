# 9. Main becomes the v1 line; bash patches come from release/0.11

- Status: Accepted
- Date: 2026-09-28

## Context

ADR 1 allows breaking changes in v1 (executable hooks, bundle removal, a new lock format). The hybrid images of ADR 8 need somewhere to land, while nas, vps, and do-volume-ops keep backing up nightly on the bash release.

## Options

1. Main ships `1.0.0-alpha.N` images and takes breaking changes whenever they are ready; bash patch releases come from a `release/0.11` branch.
2. Main ships each hybrid as a compatible 0.x release that the deployments take, and every breaking change waits for one 1.0.0 cutover.

## Decision

Option 1.

## Consequences

- Before either line cuts a release, the release and publish workflows gain a prerelease path for `1.0.0-alpha.N` and a patch path from `release/0.11`. Today they accept only numeric patch, minor, and major bumps, only from main, and publish every release as stable. An alpha is marked as a prerelease and leaves the image references in `compose.yaml`, the README, and the guides on 0.11.x.
- `release/0.11` branches from the 0.11.0 tag and carries only bash patch releases, each starting with a failing integration test (ADR 3).
- Alpha images are for tests and throwaway deployments only; nas, vps, and do-volume-ops stay on 0.11.x until 1.0.0.
- A fix made on `release/0.11` is checked against main, where it is ported or recorded as superseded.
- Hybrids need not reproduce 0.11 behavior beyond the never-break list, so breaking changes land with their own migration and upgrade-guide entry instead of in one batch.
