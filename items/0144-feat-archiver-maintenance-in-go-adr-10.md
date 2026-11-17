# #144 feat: archiver maintenance in Go (ADR 10)

- Kind: pull, state: closed, by cody, created 2026-10-05T11:35:03-07:00
- Labels: none; milestone: none
- refs/pull/144/head -> main, merged: True

Ports lib/scripts/maintenance.sh + lib/features/duplicacy-maintenance.sh to internal/maintenance: lock (bash format, flock), maintenance log/rotation, per-storage check and prune with the exhaustive interval (1h grace) and the .maintenance-state record, local-only while copy workers own the storages (fingerprinted daemon calls, forced exhaustive forwarded), primary prune via localprune in-process (Dir/Env options added), local-changed only after a prune that deleted revisions, stop flag and SIGTERM end the current command, same messages and exit codes. Full suite passes (maintenance, stop-skips-prune, copy-workers, prune-in-use, sigterm-stop, e2e maintenance tests).
