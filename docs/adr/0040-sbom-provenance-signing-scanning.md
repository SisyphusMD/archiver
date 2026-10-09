# 40. Images carry an SBOM and provenance, releases are signed, scans report

- Status: Accepted
- Date: 2026-10-08

## Context

Published images say nothing about what they contain or how they were built, and nothing
proves a pulled image came from this project's CI. Known vulnerabilities in the image's
packages go unreported.

## Options

For each of an SBOM, build provenance, signing and vulnerability scanning: on or off, and
whether a failure blocks a release.

## Decision

SBOM and provenance always on; release signing enforced; vulnerability scanning advisory
(Cody accepted this after an explanation of the trade-offs).

## Consequences

- Every published image carries an SBOM and a provenance attestation (buildx); neither can
  fail a build.
- Release images are signed with cosign, with the key in OpenBao and a Forgejo secret; a
  release whose signing fails stops rather than ship unsigned.
- trivy scans each release and runs weekly, reporting known vulnerabilities without
  blocking: most come from Debian packages with no fix available.
