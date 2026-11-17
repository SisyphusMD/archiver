# #322 fix(status): take a secondary's last check and prune from its copy worker

- Kind: pull, state: closed, by cody, created 2026-11-05T06:41:57-08:00
- Labels: none; milestone: none
- refs/pull/322/head -> main, merged: True

Maintenance keeps to local while copy workers check and mirror the secondaries (ADRs 12, 17, 18), so status, the web page and `archiver_storage_last_{check,prune}_timestamp_seconds` kept showing each secondary's last maintenance run from before the workers took over: on the NAS, cowanserver and backblaze read as unpruned since 10/14 although both mirror daily.

A secondary now takes the later of maintenance's record and its worker's last check, mirror pass (as its prune) and exhaustive prune. healthcheck already skipped worker-kept secondaries and is unchanged.

Test: TestSecondaryMaintenanceFromWorker. Full unit suite with -race; Codex review clean.
