# #237 fix(ci): repin the Podman test host to quay's current v5.8.7 digest and let Renovate track it (quay garbage-collected the old digest; tests/integration/podman.sh was outside Renovate's paths)

- Kind: pull, state: closed, by cody, created 2026-10-13T08:22:44-07:00
- Labels: none; milestone: none
- refs/pull/237/head -> main, merged: True

quay re-pushed v5.8.7 on 2026-10-13 and dropped the pinned digest, so Host-driven integration fails on every PR. Repins and adds the script to Renovate's includePaths and the inline-image manager.
