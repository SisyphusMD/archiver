# #42 ci: gate arm64 emulation build on Dockerfile changes

- Kind: pull, state: closed, by cody, created 2026-07-05T09:30:33-07:00
- Labels: none; milestone: none
- refs/pull/42/head -> main, merged: True

pr-validate only runs the slow arm64-under-qemu build when the PR touches the Dockerfile (arch-specific downloads / base+duplicacy bumps live there). Arch-independent PRs (code/tests/docs) skip it, cutting PR CI from ~20 min to ~5. arm64 is still validated at release by build-and-push multi-arch. Fail-safe: defaults to running arm64 if the base cannot be fetched/diffed. This PR does not touch the Dockerfile, so its own pr-validate should skip arm64.
