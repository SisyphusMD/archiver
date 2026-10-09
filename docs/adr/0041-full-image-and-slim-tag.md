# 41. The full image stays, and a slim tag is published beside it

- Status: Accepted
- Date: 2026-10-08

## Context

The image is about 1 GB, mostly tools hooks may call: systemd's systemctl, zfsutils,
btrfs-progs, Python with lmdb, ssh, curl, sqlite3, editors. A deployment whose hooks need
none of them still pulls them all.

## Options

1. Keep the tools, drop the editors and ping.
2. Keep the full image as the default and also publish a slim tag.
3. Keep the image as it is.

## Decision

Option 2.

## Consequences

- `:<version>` is unchanged, with every tool hooks may use.
- `:<version>-slim` keeps everything Archiver itself runs (among them tini, duplicacy,
  rclone, openssl, ssh, curl, qrencode, ca-certificates and tzdata) and drops only tools
  that hooks alone use; it is built, tested and released alongside.
- The README says which tools each tag has.
