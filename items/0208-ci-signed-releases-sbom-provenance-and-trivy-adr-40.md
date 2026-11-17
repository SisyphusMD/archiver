# #208 ci: signed releases, SBOM, provenance and trivy (ADR 40)

- Kind: pull, state: closed, by cody, created 2026-10-09T21:44:54-07:00
- Labels: none; milestone: none
- refs/pull/208/head -> main, merged: True

Stacked on #207 (lint/fuzz): merge that first.

ADR 40, release path only (build-and-push runs on tags):
- Build with `sbom: true` and `provenance: mode=max`; if the attested build fails, a second build without attestations runs and warns (attestations never fail a release).
- The OCI archive is pushed to one candidate tag (`release-candidate`); skopeo --digestfile records exactly what was pushed. cosign v3.0.2 signs that digest with the key from secrets (COSIGN_PRIVATE_KEY/COSIGN_PASSWORD), `--use-signing-config=false --tlog-upload=false` (no Rekor; signature stored beside the image as the `sha256-<hex>` tag), verifies it against cosign.pub, and only then are the release tags set (copied from the signed digest). A missing secret or failed signature fails the job, so tags, mirrors and release notes never carry an unsigned image. Registry login goes to cosign through a per-run volume's config.json, not argv.
- NAS sync carries the signature tag; the GHCR mirror copies it explicitly.
- trivy 0.69.1 scans each release (continue-on-error) and the newest release weekly (scan-weekly.yml, SemVer-aware newest, exit 0).
- cosign.pub at the repo root; README "Verifying Images"; .renovaterc includePaths gains scan-weekly.yml; per-run volume names (also for the existing skopeo-stage).

Key: cosign key pair in OpenBao kvv2 projects/archiver/release-signing (version 2) and the two Forgejo repo secrets. Version 1 was retired before any use: a local dry run with cosign v3's default signing-config mode uploaded two test entries (alpine:3.20's digest, signed with that key) to public Rekor despite --tlog-upload=false; nothing secret was in them. The fix (--use-signing-config=false) was proven with a throwaway key: no Rekor entry.

Not exercisable before a real tag: the first release run is the first live signing. Codex: 6 rounds, last clean.
