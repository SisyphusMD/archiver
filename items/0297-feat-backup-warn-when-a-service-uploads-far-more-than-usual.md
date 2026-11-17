# #297 feat(backup): warn when a service uploads far more than usual

- Kind: pull, state: closed, by cody, created 2026-10-29T04:53:15-07:00
- Labels: none; milestone: none
- refs/pull/297/head -> main, merged: True

Each service keeps its last seven successful uploads (logs/.backup-state.json, recent_uploaded). An upload at least 4x their median and over 20 GiB more logs a WARNING and raises an "Unusually Large Backup" problem notification; a normal upload later clears it. Came from the NAS Garage incident: a path change made every file new and re-uploaded ~500-690 GB on two nights with every backup green.
