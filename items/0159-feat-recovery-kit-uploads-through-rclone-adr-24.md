# #159 feat: recovery kit uploads through rclone (ADR 24)

- Kind: pull, state: closed, by cody, created 2026-10-06T07:26:37-07:00
- Labels: none; milestone: none
- refs/pull/159/head -> main, merged: True

ADR 24: the recovery kit reaches every storage through a bundled, checksum-pinned rclone (v1.75.1, Renovate-tracked) instead of per-type curl/sftp code.

- Credentials go to rclone only in its environment (`RCLONE_CONFIG_KIT_*`, `--config /dev/null`), never argv or disk.
- The kit is placed only beside an existing storage (its Duplicacy `config` must be at the destination): rclone would otherwise create a missing directory and report success for an unmounted path or mistyped bucket.
- `--ignore-times`: a kit chosen for upload is always sent (equal size and coarse timestamps would skip a changed kit and leave the old one marked current).
- SFTP: `set_modtime=false` (servers refusing SETSTAT), no remote shell, host keys trusted on first use as before, and only the key types already trusted are offered (otherwise rclone negotiates another type and calls it a changed key).
- Local and SFTP permission matching is kept (owner/mode of the storage's config, chmod only when it differs, verified or kept out of the state).
- Existing kits stay current: same content, name, state.

Tests (each new one confirmed failing without its fix): sftp-runtime gains a server refusing SETSTAT and a known_hosts with one of several key types; unit tests for credentials off argv, local placement, trusted algorithms. Local: all integration tests, full e2e.
