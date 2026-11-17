# #337 fix(doctor): no kit warning when only the extra files changed since the backup

- Kind: pull, state: closed, by cody, created 2026-11-13T11:47:44-08:00
- Labels: none; milestone: none
- refs/pull/337/head -> main, merged: True

On the NAS, RECOVERY_KIT_EXTRA_PATHS holds files that change daily (DSM config, git bundles), so `archiver doctor` warned that the kit was not yet placed for most of every day.

The kit now also records a core fingerprint (the same hash without extra/) in logs/.recovery-kit-core. Doctor says OK, naming the storages, when a storage holds a kit whose core is current and only extras changed; it still warns when settings, keys or notes changed. The full fingerprint and its state file are unchanged, so no kit is re-uploaded. Storages a refresh leaves alone keep their place on the core record, and a run with nothing to upload still writes it.

Tests: new phase in tests/integration/doctor.sh (fails on main), unit tests for both review findings.
