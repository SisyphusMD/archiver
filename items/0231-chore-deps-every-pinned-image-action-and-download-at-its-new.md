# #231 chore(deps): every pinned image, action and download at its newest version or digest

- Kind: pull, state: closed, by cody, created 2026-10-11T21:15:05-07:00
- Labels: none; milestone: none
- refs/pull/231/head -> main, merged: True

Everything pinned in the repo, at the newest version or digest (2026-10-11):

- Dockerfile: Debian trixie-20261005-slim (same as #229), docker CLI 29.9.0 with checksums (supersedes #230 and Renovate #72). Duplicacy 3.2.5, rclone 1.75.2 and the Go module are already newest.
- Workflows: trivy 0.75.0, cosign 3.1.3 (signs and verifies with --use-signing-config=false --tlog-upload=false in a network with no internet; only a deprecation warning), skopeo v1.22.3 new digest, build-push-action 7.4.0, login-action 4.6.0, setup-buildx-action 4.4.1, setup-qemu-action 4.4.0 (supersedes Renovate #213 #214 #97 #70 #77 #87).
- Tests: golang 1.27.2 (e2e), docker 29.9.0-cli, caddy 2.11.7, garage v2.4.1, podman v5.8.7, postgres 18.6 and redis 8.10.2 for the Storj stand-in (postgres 18 needs its tmpfs at /var/lib/postgresql), new digests for atmoz/sftp:alpine and openstackswift/saio:latest.
- Not touched: the archiver 0.11.0/0.11.4 pins in tests/e2e (cross-version data).

Locally: backends storj + minio, s3-garage pass.
