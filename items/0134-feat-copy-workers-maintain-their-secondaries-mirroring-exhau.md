# #134 feat: copy workers maintain their secondaries (mirroring, exhaustive prune, per-target checks)

- Kind: pull, state: closed, by cody, created 2026-10-02T21:52:38-07:00
- Labels: none; milestone: none
- refs/pull/134/head -> main, merged: True

ADRs 12, 17, 18. Workers mirror local retention onto each secondary with the ADR 18 rails, run the exhaustive prune on PRUNE_EXHAUSTIVE_FREQUENCY, and check each target on its own interval when idle. Bash maintenance keeps to local while workers run and wakes them after a prune that deleted revisions. `archiver mirror --dry-run` / `--allow-large`. Worker repositories move to logs/.copy-repos, keyed by storage identity, so fossil collections survive restarts.
