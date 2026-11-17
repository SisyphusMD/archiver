# #209 feat: -slim image tag (ADR 41)

- Kind: pull, state: closed, by cody, created 2026-10-10T01:10:38-07:00
- Labels: none; milestone: none
- refs/pull/209/head -> main, merged: True

Stacked on #208 (signing) on #207 (lint): merge those first.

ADR 41.
- Dockerfile: a `slim` stage with everything Archiver runs (tini, OpenSSH client, OpenSSL, curl, ca-certificates, tzdata, qrencode, plus the archiver/duplicacy/rclone binaries and all image config), and the final `full` stage (default target) adding the hook tools: docker CLI, systemd, zfsutils, btrfs-progs, python3 + lmdb, sqlite3, procps, nano, vim, ping. 337 MB vs 1.11 GB.
- Release: build-and-push becomes a matrix over full/slim, max-parallel 1 (shared layers race Forgejo's blob uniqueness; both run the NAS sync); slim tags carry `-slim` (metadata-action suffix), its own candidate tag and per-variant volumes; signed like the full image. Mirror outcomes are set only on failure so one leg cannot overwrite the other's.
- PR CI: new `slim` job builds the slim target and runs tests/integration/slim-tools.sh (SLIM=1: essentials present, hook tools absent) and roundtrip, recovery-kit, envelope, restore-commands, maintenance, recover; integration checks the full image holds the hook tools (SLIM=0). build-and-push needs it.
- scan-weekly skips -slim tags when picking the newest release. README "Image Tags".

Local: slim-tools both ways, six scripts on slim, and all 52 integration scripts + e2e on the new full image pass (its essentials now come from the slim stage without recommends). Codex: 2 rounds, last clean.
