# #51 test: comprehensive integration + bats suite, and the five bugs it surfaced

- Kind: pull, state: closed, by cody, created 2026-07-09T15:36:46-07:00
- Labels: none; milestone: none
- refs/pull/51/head -> main, merged: True

Comprehensive test build-out plus the five bugs writing it surfaced.

**New integration tests (13).** In-container: copy-path (secondary storage receives the backup, restores from it, and a failing `duplicacy copy` is reported), multi-service (glob expansion, one wrap-up, per-service failure isolation), lock-concurrency (busy refusal, stale-lock recovery), pause-resume (pause freezes duplicacy to state T, resume completes, stop-while-paused kills), stop-nonfinal (stop honored at the next service boundary), service-settings (filters shape the snapshot; hooks ordered and isolated), healthcheck (four states), restore-extras (revision pinning, IGNORE_OWNERSHIP, restore-service.sh hook). Host-driven, in a new parallel `integration-host` CI job: sigterm-stop (real `docker stop` mid-backup exits gracefully in seconds, no prune), invalid-cron (fail-fast), s3-minio (backup+restore over the s3 wire against a TLS MinIO sidecar — virtual-host addressing, exactly like Spaces/AWS), sftp-runtime (backup+restore over ssh against an sshd sidecar). The storage sidecar tests exercise the exact production `build_storage_url` + `DUPLICACY_<NAME>_*` code paths that string-level tests missed in 0.8.10/0.8.11.

**New bats (36 → 64):** lockfile.sh semantics, SERVICE_DIRECTORIES glob expansion, config-serialize round-trips (metacharacter-heavy secrets byte-exact through both output shapes), format_duration.

**Fixes the tests surfaced:**
1. `acquire_lock` stale recovery removed the dead lock but never re-took it — the recovered run was lockless (stop/status/pause blind; a concurrent backup would be admitted). It now re-acquires.
2. A backup refused because one is already running now explains itself instead of exiting 1 silently.
3. `archiver healthcheck` reported healthy env-native deployments (no config.sh) permanently UNHEALTHY, and its finished-run detection grepped a log line nothing emits ("Archiver script completed"); it now recognizes env-native mode and matches the real end-of-run marker.
4. The entrypoint SIGTERM handler exited immediately after `archiver stop`, racing PID-namespace teardown against the backup recording its stop — it now waits (bounded under the 2m grace period) for the lock to clear.
5. Env-native SFTP could back up but not restore: `ssh_public_key` was never placed by the entrypoint nor exported by `archiver migrate`, and the restore path requires both key halves. Docs updated (README, compose.yaml).

Verified locally against the built image: 16/16 in-container integration, 4/4 host-driven, 64/64 bats, entrypoint smokes. The release gate now also requires the `integration-host` job.
