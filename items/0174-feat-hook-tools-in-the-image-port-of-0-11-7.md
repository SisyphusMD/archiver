# #174 feat: hook tools in the image (port of 0.11.7)

- Kind: pull, state: closed, by cody, created 2026-10-08T04:43:00-07:00
- Labels: none; milestone: none
- refs/pull/174/head -> main, merged: True

Port of #173 to main: `btrfs-progs`, `python3`, `python3-lmdb` in the image, the `hook-tools.sh` integration test wired into CI, and SYS_ADMIN documented as the btrfs opt-in (README, commented in compose.yaml). Locally: hook-tools and round-trip green on the main image.
