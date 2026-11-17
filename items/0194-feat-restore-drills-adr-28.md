# #194 feat: restore drills (ADR 28)

- Kind: pull, state: closed, by cody, created 2026-10-09T08:27:43-07:00
- Labels: none; milestone: none
- refs/pull/194/head -> main, merged: True

RESTORE_DRILL_SCHEDULE (daemon job), RESTORE_DRILL_SERVICES (1 rotating per storage, or all), _STORAGES (all|primary|names), _DIR (default /tmp/archiver-drill), _EXCLUDE. Each drill: fresh scratch dir (never a pre-existing path; a drill dir overlapping service data is refused), revision chosen and held in use under the storage gate, duplicacy restore verifies chunks, file count checked against the listed file lines (the Files: summary is missing for empty files), copy deleted, no hooks. Failed drill notifies (Restore Drill Failed); Restore Drill Complete is routine. status shows last drill per service/storage; healthcheck warns on a failed drill or none passing within 2x the interval (clock from first scheduling). stop/pause/resume act on drills, including while waiting on locks. archiver drill [SERVICE] [STORAGE]. Also: restores now take the storage-creation lock around init. tests/integration/restore-drill.sh (real duplicacy: rotation, damaged chunks fail, offsite passes). All 36 integration scripts and e2e pass locally.
