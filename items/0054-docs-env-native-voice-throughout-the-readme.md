# #54 docs: env-native voice throughout the README

- Kind: pull, state: closed, by cody, created 2026-07-10T09:18:17-07:00
- Labels: none; milestone: none
- refs/pull/54/head -> main, merged: True

Second docs pass on owner feedback: the README still carried bundle-first framing beyond the operational examples. Changes: the init flow now mounts a neutral `./archiver-setup` output directory (the bundle is one of the artifacts written there, not the identity of the flow); the mode table calls init "setup" not "bundle generation" and lists env-native before bundle in the daemon/run rows; the Configuration intro and the renamed "Configuration Sources: Env-Native or Bundle" section (anchor updated everywhere) lead with env-native as the primary mode with the bundle as the optional transitional baseline / DR escrow; Service Directories documents the canonical colon-delimited form with the bash array noted as legacy; init's own output text says "setup directory" and "disaster-recovery bundle". init integration test re-verified against the rebuilt image.
