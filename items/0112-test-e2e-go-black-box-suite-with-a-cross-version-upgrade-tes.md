# #112 test(e2e): Go black-box suite with a cross-version upgrade test, gated in CI

- Kind: pull, state: closed, by cody, created 2026-09-28T19:34:02-07:00
- Labels: none; milestone: none
- refs/pull/112/head -> main, merged: True

First Phase 4 chunk (ADR 2): permanent black-box tests in Go, driving images via docker.

TestCrossVersionUpgrade: 0.11.0 (pinned digest) backs up to a primary and a bit-identical offsite copy; the image under test takes over the same installation, adds revision 2 to the same snapshot ID on both, restores revisions 1 and 2 from both (content, mode, owner, symlinks), 0.11.0 restores the new revision (rollback), and every kit on every storage opens with stock openssl and carries the keys and storage password.

Proven able to fail against four sabotaged images (changed snapshot ID, ownership-dropping restore, non-stock kit, non-bit-identical copy). Codex review clean.

Second commit adds the e2e CI job and makes build-and-push wait on it.
