# #357 Retire the old release dispatcher on the 0.11 line

- Kind: pull, state: closed, by cody, created 2026-11-17T19:17:09-08:00
- Labels: none; milestone: none
- refs/pull/357/head -> release/0.11, merged: True

Closes #356.
Depends on #355 (merged).

Replace the maintenance dispatcher that pushed the branch and tag directly with instructions to use the trusted main dispatches targeting release/0.11. The new maintenance path is verified: v0.11.9 went through Prepare Release (run 858, PR #376) and Publish Release (run 41557) from main and published (run 41558, digest sha256:5e6b551c on ghcr, cluster and NAS registries).

No tests: The replacement workflow contains only a diagnostic and fails immediately. Strict YAML lint and independent review passed.
