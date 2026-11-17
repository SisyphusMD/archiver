# #106 test: run the s3 integration test against Garage instead of MinIO

- Kind: pull, state: closed, by cody, created 2026-09-26T16:08:41-07:00
- Labels: none; milestone: none
- refs/pull/106/head -> main, merged: True

MinIO's community image (`minio/minio`) is gone from Docker Hub, so `tests/integration/s3-minio.sh` fails in the host-driven CI job on every PR (including Renovate's and #105), and the sftp and entrypoint-secrets tests after it never run.

This swaps the fixture for Garage v2.3.0 (the NAS's exact pin), single-node with a caller-chosen key and `root_domain` for virtual-host buckets, behind a Caddy 2.11.4 sidecar that terminates TLS (duplicacy's `s3://` backend is https-only). Same production path and the same assertions as before: backup over `s3://`, the recovery kit as a plain object readable with SigV4 path-style, and restore with content checks. Readiness waits for a signed ListObjectsV2 to return 200.

Verified on a real Docker daemon: s3-garage passes (38s), and so do the other host-driven tests (sigterm-stop, invalid-cron, sftp-runtime, entrypoint-secrets) plus roundtrip. Test-only change; no CHANGELOG entry.
