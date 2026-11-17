# #243 feat(drill): named drill schedules (ADR 53)

- Kind: pull, state: closed, by cody, created 2026-10-13T18:02:05-07:00
- Labels: none; milestone: none
- refs/pull/243/head -> main, merged: True

Two commits: ADR 53 (text approved by Cody), and its implementation.

RESTORE_DRILL_<NAME>_SCHEDULE defines a named drill; _SERVICES (all of them each run), _STORAGES (all | primary | names | alternate | alternate:NAME,NAME: one storage per run, in turn, cursor persisted, held on a storage until a run there passes), _TIMEOUT. Each named drill is its own kind under ADR 52 (due until it passes, time limit), with its own state, incidents, status lines (also on the HTML status page, with deep-check and kit-drill results) and metrics: archiver_restore_drill_* gains a drill label (default or the name; new series identities), archiver_drill_* aliases unchanged. NOTIFY_ON -drills covers them. Drill hooks run as usual. Use case: a whole object store drilled monthly from offsite storages in turn, excluded from the regular rotation.

Tests: config parsing/validation (name rule vs existing keys), alternate and alternate:LIST rotation through failures/restarts/list changes, metrics labels, status and web rendering, opt-out; restore-drill.sh named case; metrics, incidents, invalid-cron, status page scripts. Two Codex review rounds; findings fixed.
