# #163 ci: real-service backends weekly and before every release (ADR 27)

- Kind: pull, state: closed, by cody, created 2026-10-06T14:34:15-07:00
- Labels: none; milestone: none
- refs/pull/163/head -> main, merged: True

ADR 27 (Cody approved 2026-10-06) supersedes ADR 25's nightly schedule:

- `real-backends.yml` (was `nightly-backends.yml`) runs weekly (Mondays 06:17 UTC) and on demand.
- `release.yml` runs the same round trips after the smoke test, before anything is tagged; a failure stops the release. Until the test accounts exist (`NIGHTLY_RSA_KEY` unset) it warns and continues.
- `skip_real_backends` lets a release proceed through an outside service's outage, and adds a `### Notes` line to that release's CHANGELOG section saying so.
- Both workflows share the `release` concurrency group, so two runs never back up the shared test snapshots at once.
