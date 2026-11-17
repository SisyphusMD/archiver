# #105 docs(adr): v1.0.0 compatibility policy and Phase 1 gate decisions

- Kind: pull, state: closed, by cody, created 2026-09-26T07:05:32-07:00
- Labels: none; milestone: none
- refs/pull/105/head -> main, merged: True

Records the ground-up refactor's compatibility policy and the four Phase 1 gate decisions, one ADR each:

- 0001: v1.0.0 compatibility policy. Never break storages, snapshot IDs, encryption keys, restoring old snapshots, or plain-openssl kit recovery; everything else may change with a migration command and an upgrade guide.
- 0002: cross-version compatibility tests before rewrite code.
- 0003: patch the audit's safety findings in bash first, each with a failing test.
- 0004: drop bundle mode in v1, after a tested migration.
- 0005: refactor backlog scope.

Docs only; no runtime change and no CHANGELOG entry (not user-facing).
