# #117 docs(adr): offsite-resilience decisions (ADRs 0011-0013)

- Kind: pull, state: closed, by cody, created 2026-09-29T15:55:58-07:00
- Labels: none; milestone: none
- refs/pull/117/head -> main, merged: True

Cody's decisions of 2026-09-29, after the NAS skipped the 09-28 backup behind a long copy, an ISP outage failed both copies with no retry, and local accumulated 2.17 TiB of orphaned fossils from ghost-snapshot races.

- 0011: a copy worker per target (targets in parallel, one all-snapshot copy at a time per target, backoff retries until caught up; backup lock covers only the local backup).
- 0012: targets mirror local retention (only local runs -keep; guarded deletions on targets).
- 0013: built in the Go pipeline port next, with a refused-backup alert in bash meanwhile.

Text approved by Cody; Codex review surfaced two real gaps (mirroring must also trigger on local prunes; bash maintenance must stop pruning targets when mirroring ships), both folded in. A third Codex claim (per-ID partial visibility in an all-ID copy) was rejected: duplicacy_main calls CopySnapshots once and it writes all snapshot files after all chunks.
