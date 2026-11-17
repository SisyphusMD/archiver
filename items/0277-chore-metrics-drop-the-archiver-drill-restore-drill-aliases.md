# #277 chore(metrics): drop the archiver_drill_* restore drill aliases

- Kind: pull, state: closed, by cody, created 2026-10-24T08:46:12-07:00
- Labels: none; milestone: none
- refs/pull/277/head -> main, merged: True

The aliases were kept for one release (rc1) after the restore drill metrics moved to archiver_restore_drill_* with a drill label. No alert rule or dashboard in the deployment repos reads them. Unit and integration tests now assert they are gone; docs and CHANGELOG updated.
