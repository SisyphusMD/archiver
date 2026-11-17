# #108 docs(adr): Phase 2 decisions (Go gated on a slice; pinned duplicacy binary)

- Kind: pull, state: closed, by cody, created 2026-09-27T12:40:32-07:00
- Labels: none; milestone: none
- refs/pull/108/head -> main, merged: True

- 0006: Go for v1, committed only after a throwaway vertical slice proves it (backup + copy to test storages, cancel duplicacy without skipping post hooks, circuit-breaker state across restart, cross-version restore).
- 0007: keep executing the checksum-pinned duplicacy CLI instead of importing its Go package (license, panic-based errors and os.Exit, global state, stale deps).

Docs only.
