# #242 feat(drill): due until it passes, time limits, one metric naming, and a NOTIFY_ON opt-out for drill results (ADR 52)

- Kind: pull, state: closed, by cody, created 2026-10-13T14:06:52-07:00
- Labels: none; milestone: none
- refs/pull/242/head -> main, merged: True

- Restore drills, deep checks and kit drills stay due after a failure or an interruption (stop, timeout) and run again at the next scheduled time until one passes; genuine skips (no revision yet, too little space) still let rotation advance.
- RESTORE_DRILL_TIMEOUT ends a restore drill like a stop.
- Metrics: archiver_restore_drill_last_timestamp_seconds, _last_ok, _last_duration_seconds, _last_success_timestamp_seconds per storage and service, matching archiver_deep_check_* and archiver_kit_drill_*; archiver_drill_last_ok and archiver_drill_last_timestamp_seconds kept as deprecated aliases.
- NOTIFY_ON=failures,-drills (also per destination) leaves drill results out of notifications; incidents, status, health and metrics still record them. Default unchanged: drill failures notify.

Tests: unit tests for each (pending vs skip vs interruption, timeout, metric names and aliases, opt-out suppression and re-eligibility of undelivered alerts, failure incident raised even when the state write fails); restore-drill.sh, metrics.sh, incidents.sh, deep-check.sh, kit-drill.sh rerun. Three Codex review rounds; findings fixed.
