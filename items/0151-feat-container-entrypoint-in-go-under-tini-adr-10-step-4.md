# #151 feat: container entrypoint in Go under tini (ADR 10 step 4)

- Kind: pull, state: closed, by cody, created 2026-10-05T17:00:39-07:00
- Labels: none; milestone: none
- refs/pull/151/head -> main, merged: True

ADR 10 step 4, part 1: the entrypoint moves from `docker-entrypoint.sh` (which sourced the bash library) to `archiver entrypoint` (`internal/entrypoint`), with `tini` as PID 1.

- Same bundle refusal, key placement, stale-lock clearing, CRON_SCHEDULE/ROTATE/MAINTENANCE warnings, schedule check, `init` and `run <command>` modes, log forwarding (`tail -F -n 0` semantics, rotation and truncation followed), and graceful SIGTERM (stop, wait for both locks up to 100s, then end the daemon).
- `tini` because a Go PID 1 does not reap processes hooks orphan, and a home-made reaper would race the exit statuses Go collects from its own children. Checked: no zombies after orphaning processes in the container; `docker stop` exits 0.
- An `entrypoint:` override naming `/usr/local/bin/docker-entrypoint.sh` must drop it (CHANGELOG); nas and vps use the image default.

Local: unit tests (-race), sigterm-stop, invalid-cron, refuse-bundle, s3-garage, sftp-runtime, full e2e suite (every container starts through the entrypoint).
