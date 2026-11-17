# #238 docs(adr): 48-52, the drill gate

- Kind: pull, state: closed, by cody, created 2026-10-13T11:14:02-07:00
- Labels: none; milestone: none
- refs/pull/238/head -> main, merged: True

Cody's decisions of 2026-10-12/13 for folding the NAS archiver-drill container into Archiver, text approved verbatim (including two later revisions: ADR 50 resets verified-chunk progress after a completed check; ADR 51 covers OneDrive, whose envelope cannot hold a token):

- 48 a per-service drill hook proves a restored service is usable
- 49 a drill checks every restored file against the revision's record (supersedes ADR 28's file-count check)
- 50 a scheduled deep check reads every chunk of a storage
- 51 a kit drill rehearses recovery from the envelope alone
- 52 per-kind schedules (due until it passes, time limits), one metric naming on METRICS_PORT, drill notifications on by default with a NOTIFY_ON opt-out
