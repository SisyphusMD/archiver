# #227 fix(security): Duplicacy repositories out of service directories (scan F7, F8, F9, F10)

- Kind: pull, state: closed, by cody, created 2026-10-11T10:37:16-07:00
- Labels: none; milestone: none
- refs/pull/227/head -> main, merged: True

Security scan 2026-10-11, findings F7, F8, F9, F10: Duplicacy ran as root from each service directory, so its repository (preferences, filters, cache, resume points) lived where the service's owner could change it.

- Backups run from logs/.service-repos/<snapshot id>-<sha256(abs dir)[:12]>/ (owner-only, refused if a link, foreign-owned or group/other-writable), with init -repository <service dir>. Snapshot IDs and storages unchanged; cross-version e2e passes. Leftover .duplicacy directories in service directories are skipped by Duplicacy and left alone.
- A failed init fails the service instead of backing up with whatever preferences are there.
- Restores and drills run from a private temp repository; a killed drill's is cleaned up through its run record (validated: temp dir, name prefix, owned, not a link).
- Maintenance adopts legacy fossil collections only from an Archiver-owned tree, walked with openat/O_NOFOLLOW, regular files with one link.
- e2e harness: containers removed with their volumes (it leaked one anonymous volume per container).

Behaviour change: with the logs volume full, backups stop (loudly) until there is room, since each service's repository lives there; documented in upgrading.md. Validation: go test -race as root, golangci-lint 0, all roundtrip-job integration scripts incl. new service-repos.sh, backends (several types), e2e.
