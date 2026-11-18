# #357 WIP: Retire the old release dispatcher on the 0.11 line

- Kind: pull, state: open, by cody, created 2026-11-17T19:17:09-08:00
- Labels: none; milestone: none
- ci/retire-legacy-release-writer -> release/0.11, merged: False

Closes #356.
Depends on #355.

Replace the maintenance dispatcher that pushed the branch and tag directly with instructions to use the trusted main dispatches targeting release/0.11. Keep this draft until #355 merges and the new maintenance path is verified.

No tests: The replacement workflow contains only a diagnostic and fails immediately. Strict YAML lint and independent review passed.
