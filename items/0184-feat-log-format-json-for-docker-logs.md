# #184 feat: LOG_FORMAT=json for docker logs

- Kind: pull, state: closed, by cody, created 2026-10-08T16:32:56-07:00
- Labels: none; milestone: none
- refs/pull/184/head -> main, merged: True

Phase 4 structured logging. LOG_FORMAT=json turns every docker logs line (the forwarded archiver/maintenance/copies logs and the entrypoint own output) into {"time","level","service","log","msg"}; the files under logs/ keep the text format. One-shot modes (init, migrate, run) stay text. tests/integration/json-logs.sh (host-driven job). Ran all 29 round-trip scripts and the host scripts locally against the built image.
