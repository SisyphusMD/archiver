# #371 fix: refresh the Debian base image on the 0.11 line

- Kind: pull, state: closed, by cody, created 2026-11-20T17:29:06-08:00
- Labels: none; milestone: none
- refs/pull/371/head -> release/0.11, merged: True

Closes #370.

Maintenance patch for the 0.11 line: `debian:trixie-20260112-slim` to `debian:trixie-20261104-slim`, the same multi-arch digest main builds on. The pinned downloads (Duplicacy, supercronic, Docker CLI) are unchanged; apt packages come from the newer snapshot. Changelog entry under `## [Unreleased]` (Security).

Tests: no code change. The PR build, smoke, integration and never-break suites on this branch are the regression proof for the new snapshot.

Merging this does not release anything. v0.11.8 goes through the trusted-main Prepare/Publish Release dispatches only after Cody approves it.
