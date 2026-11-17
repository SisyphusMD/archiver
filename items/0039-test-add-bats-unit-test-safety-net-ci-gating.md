# #39 test: add bats unit-test safety net + CI gating

- Kind: pull, state: closed, by cody, created 2026-07-04T20:21:45-07:00
- Labels: none; milestone: none
- refs/pull/39/head -> main, merged: True

Adds a bats unit-test suite for config-loader (sanitize_storage_name incl. both do-spaces regressions; count_storage_targets contiguity; check_required_secrets) and a bats CI job wired into the needs: of pr-validate and build-and-push, so tests gate PRs and block releases. Verified locally: 12/12 green under bats 1.13.0.
