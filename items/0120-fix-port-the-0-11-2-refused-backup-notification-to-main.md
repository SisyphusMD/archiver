# #120 fix: port the 0.11.2 refused-backup notification to main

- Kind: pull, state: closed, by cody, created 2026-09-29T16:59:04-07:00
- Labels: none; milestone: none
- refs/pull/120/head -> main, merged: True

ADR 9 port of #119 (released as 0.11.2 from release/0.11): the failing-first test and the main.sh notification, cherry-picked. main's backup pipeline is still bash, so both apply unchanged. CHANGELOG gains the released [0.11.2] section below Unreleased, so release reconciliation from main tags keeps the 0.11.2 notes.

Local on the built main image: TestRunsRefusedBackupNotifies, TestRunsSecondBackupRefused, TestCrossVersionUpgrade pass.
