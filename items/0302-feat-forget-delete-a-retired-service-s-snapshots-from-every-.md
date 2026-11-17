# #302 feat(forget): delete a retired service's snapshots from every storage

- Kind: pull, state: closed, by cody, created 2026-11-01T07:22:46-08:00
- Labels: none; milestone: none
- refs/pull/302/head -> main, merged: True

Nothing ever removed the snapshots of a service taken out of SERVICE_DIRECTORIES: Duplicacy's prune never deletes an ID's newest revision and passes over an ID with no revision in 7 days, and mirroring leaves alone an ID the primary lacks. The NAS kept 13 such IDs (forgejo 61 GB on B2).

- `archiver forget NAME... [--yes]` lists what each storage holds, asks, then deletes `snapshots/<id>/` via the kit's rclone remote, primary first, under each storage's in-use gate (re-listing there, skipping an ID a copy or restore reads). Refuses configured services (unmounted literal dirs included), invalid IDs, and deletes nothing unless every storage lists. Chunks go in the next exhaustive prune (two-step, safe beside backups).
- `archiver doctor` warns about this host's IDs no configured service backs up.
- `config.ServiceName` resolves a service dir like the pipeline does (doctor named `.` wrong).
- tests: unit (internal/forget, doctor), integration `tests/integration/forget.sh` in CI.
