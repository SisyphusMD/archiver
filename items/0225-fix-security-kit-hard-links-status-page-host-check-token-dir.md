# #225 fix(security): kit hard links, status page Host check, token directory owner (scan F2, F13, F14)

- Kind: pull, state: closed, by cody, created 2026-10-11T09:12:05-07:00
- Labels: none; milestone: none
- refs/pull/225/head -> main, merged: True

Security scan 2026-10-11, findings F2, F13, F14.

- F2: kit placement on local storage changes owner/mode only on a fresh one-link inode owned by Archiver; a hard link planted at the kit's name is Unverified and untouched (test fails without the fix).
- F13: internal/hostguard: the status page and /metrics answer only IP-literal hosts, localhost, the container's hostname and WEB_ALLOWED_HOSTS; anything else gets 421 (DNS rebinding). New setting WEB_ALLOWED_HOSTS in the reference and schema; docs/monitoring.md says to list a name you reach it by.
- F14: logs/.tokens is used only when owned by this process and not group/other-writable, for writes and reads. Residual: whoever owns the logs volume on the host can still swap entries mid-run; docs say to keep it root-only.
