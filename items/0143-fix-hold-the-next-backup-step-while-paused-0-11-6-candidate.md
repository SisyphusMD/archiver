# #143 fix: hold the next backup step while paused (0.11.6 candidate)

- Kind: pull, state: closed, by cody, created 2026-10-05T10:30:06-07:00
- Labels: none; milestone: none
- refs/pull/143/head -> release/0.11, merged: True

Pause only SIGSTOPped the running children, so a step started after the pause (duplicacy backup, copy, kit) ran unpaused; also the TestRunsPauseResume CI flake. update_lock_stage (every step boundary) and the add/kit steps now wait while paused; a stop ends the wait and is honored before new work (before a pre hook the service is skipped, so neither hook runs). New pause-gap integration test fails on 0.11.5; lockfile.bats covers the wait. Main has the Go equivalent (#142).
