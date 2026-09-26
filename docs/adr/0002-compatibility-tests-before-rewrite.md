# 2. Compatibility tests before rewrite code

- Status: Accepted
- Date: 2026-09-26

## Context

The refactor brief named `tests/integration/` as the parity harness. The Phase 1 audit found that those 24 tests also pin incidental details: exact log wording, the lock-file format, internal paths, fault injection by swapping the `duplicacy` binary on `PATH`, and, in one test, sourced bash functions. The 88 bats tests call bash functions directly. Under ADR 1, only the never-break list is fixed.

## Options

1. Characterize first: separate contract from incidental assertions and build a black-box suite before new code.
2. Freeze the harness as it stands and make the rewrite reproduce every assertion.
3. Keep the bash tests for bash and write a separate suite for the rewrite.

## Decision

Option 1. Before rewrite code lands:

- Build cross-version tests for the never-break list. Storages, snapshots, keys, and a recovery kit written by the current bash image must be continued (same snapshot IDs, new revisions), restored, and recovered by the new implementation.
- Treat every other current assertion as incidental unless Cody marks it contract. Contract behavior is re-expressed as black-box tests (commands, exit codes, files on storage, restore results) that do not depend on log wording or internal paths.
- Run the black-box suite against the current bash first, so the suite itself is known to be correct.

## Consequences

- Roughly a week of test work comes before new implementation code.
- The bats suite retires with the bash it tests.
- Fault injection should not depend on how Duplicacy is invoked, which keeps the Phase 2 choice between linking Duplicacy and running its binary open.
- No bash is deleted until the cross-version tests pass against v1.
