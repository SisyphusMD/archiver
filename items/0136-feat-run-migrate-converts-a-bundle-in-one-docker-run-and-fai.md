# #136 feat: run migrate converts a bundle in one docker run, and failed commands exit non-zero (0.11.4 candidate)

- Kind: pull, state: closed, by cody, created 2026-10-03T19:45:08-07:00
- Labels: none; milestone: none
- refs/pull/136/head -> release/0.11, merged: True

ADR 22: v1 points bundle deployments to `docker run ... archiver:0.11 run migrate`. The import now decrypts into a private temp dir (a read-only bundle mount works; nothing decrypted lands beside the bundle) and a restarted container re-imports its mounted bundle. Also: the archiver dispatcher exits with the command status (a failed restore used to exit 0). Each fix has an integration test that fails on 0.11.3.
